"""Standalone Python port of DocVision's level-2 vector drawing tools."""
from __future__ import annotations
import json, re, shutil, subprocess, tempfile, sys
from pathlib import Path
from PIL import Image
import numpy as np
try:
    import re2
except ModuleNotFoundError:
    vendored=str(Path(__file__).resolve().parents[2]/".deps")
    if Path(vendored).is_dir():sys.path.insert(0,vendored)
    try:import re2
    except ModuleNotFoundError as exc:raise ImportError("Go-compatible regexp support is required; install google-re2 or place it in compare-figure/.deps") from exc
try:
    from . import vector_metrics, vector_measure
except ImportError:
    import vector_metrics
    try: import vector_measure
    except ImportError: vector_measure=None

def _schema(name, description, properties, required=None):
    p={"type":"object","properties":properties}
    if required is not None:p["required"]=required
    return {"type":"function","function":{"name":name,"description":description,"parameters":p}}
def _args(a):
    if isinstance(a,str):a=json.loads(a or "{}")
    if not isinstance(a,dict):raise ValueError("tool arguments must be a JSON object")
    return a
def _safe(root,rel):
    rel=str(rel).strip()
    if rel.startswith("/"):
        parts=rel.lstrip("/").split("/",1);mount=parts[0];rel=parts[1] if len(parts)>1 else "."
        if mount!="work":raise ValueError(f"未知挂载点 {mount!r}（可用: work(rw)）")
    elif ":" in rel:
        mount,rel=rel.split(":",1)
        if mount!="work":raise ValueError(f"未知挂载点 {mount!r}（可用: work(rw)）")
    p=Path(rel)
    if p.is_absolute():raise ValueError(f"只允许相对路径: {rel}")
    q=(root/p).resolve()
    if not q.is_relative_to(root.resolve()):raise ValueError(f"路径越界: {rel}")
    return q
def _clamp(v):return max(0.0,min(100.0,float(v)))
def _compile_summary(log):
    errors=[]
    for pattern in (r"(?m)^! .*",r"(?m)^l\.\d+.*"):
        for line in re.findall(pattern,log):
            if line not in errors:errors.append(line)
    warnings=[]
    for pattern in (r"(?m)^(?:LaTeX|Package|Class|Module)\b.*Warning:.*",r"(?m)^(?:Overfull|Underfull) \\[hv]box.*"):
        for line in re.findall(pattern,log):
            line=line.strip()
            if line and line not in warnings:warnings.append(line[:300])
    err="\n".join(errors[:20]) or log[-12000:]
    warn=(f"WARNINGS ({len(warnings)}"+(f", showing first {min(20,len(warnings))}" if len(warnings)>20 else "")+"):"+"".join("\n  - "+w for w in warnings[:20])) if warnings else ""
    return err,warn
def _missing_file_hints(text,root):
    candidates=[];seen=set()
    for pattern in (r"File `([^'`\n]+)' not found",r"can't find file `([^'`\n]+)'"):
        for name in re.findall(pattern,text):
            name=name.strip()
            if not name or name in seen:continue
            seen.add(name);rel=Path(name);directory=_safe(root,str(rel.parent)) if str(rel.parent)!="." else root;best="";bestn=0
            try:entries=directory.iterdir()
            except OSError:continue
            for ent in entries:
                if not ent.is_file():continue
                n=0
                for x,y in zip(rel.name,ent.name):
                    if x!=y:break
                    n+=1
                if n>bestn:best,bestn=ent.name,n
            if best and bestn>=8 and best!=rel.name:candidates.append(f"  {name}\n    → 工作区里真实存在的是 {Path(name).with_name(best).as_posix()}（原样照抄这个）")
            if len(candidates)>=3:break
    return "MISSING FILES（名字必须原样照抄，不要手打）：\n"+"\n".join(candidates) if candidates else ""
def _catmull_resize(image,width):
    height=max(1,int(image.height*width/image.width+0.5))
    return Image.fromarray(vector_metrics._catmull_rom_resize(np.asarray(image.convert("RGB")),width,height),"RGB")
def build_standalone(body,engine_is_xe):
    pat=re.compile(r"(?m)^\\(usetikzlibrary|usepgfplotslibrary|usetikzlibrary\{arrows\}|usepackage|tikzset|pgfplotsset)\b[^\n]*")
    hoisted=[m.group(0) for m in pat.finditer(body)]
    rest=pat.sub("",body).strip();pre=([r"\usepackage{ctex}"] if engine_is_xe else [])+hoisted
    return r"\documentclass[border=6pt]{standalone}"+"\n"+r"\usepackage{tikz}"+"\n"+r"\usepackage{pgfplots}"+"\n"+r"\pgfplotsset{compat=1.18}"+"\n"+r"\usepackage{amsmath,amssymb}"+"\n"+"\n".join(pre)+"\n"+r"\begin{document}"+"\n"+rest+"\n"+r"\end{document}"+"\n"

class Harness:
    def __init__(self,root,reference,context="",threshold=17,timeout=180,**options):
        self.root=Path(root).resolve();self.root.mkdir(parents=True,exist_ok=True);self.reference=Path(reference).resolve();self.context=context or "";self.timeout=int(timeout);self.options=options
        self.threshold=float(threshold);self.min_overlap=float(options.get("min_overlap",self.threshold/100));self.max_scale_dev=float(options.get("max_scale_dev",.10));self.accept_min_submits=int(options.get("accept_min_submits",8));self.engine=options.get("engine","xelatex")
        self.current_img=options.get("current_img",self.reference.name);self.images_dir=Path(options.get("images_dir",self.reference.parent)).resolve();self._views={};self._pdf_views={};self._warned_budget=set();self._last_metrics=None
        self.state={"submitted":False,"final_code":"","last_pdf":"","merges":[],"uncertain":False,"best_overlap":0.0,"scale_dev_at_best":0.0,"accepted_as_is":False,"accept_note":"","submit_attempts":0,"compile_ok":False,"compare_called":False,"size_unit_at_best":"","aspect_ref_at_best":0.0,"aspect_cand_at_best":0.0,"ink_ref_w_at_best":0.0,"ink_ref_h_at_best":0.0,"ink_cand_w_at_best":0.0,"ink_cand_h_at_best":0.0,"best_scale_x":0.0,"best_scale_y":0.0,"compile_err":""}
    def definitions(self):
        return [
        _schema("write_file","Write a file into YOUR workspace (full content replaces the file). Allowed extensions: .cls .sty .tex .md. Keep drafts here so later edits are small diffs instead of full re-outputs.",{"path":{"type":"string","description":"relative path inside your workspace, e.g. class.cls, manual.md, examples/ch1.tex"},"content":{"type":"string","description":"full file content"}},["path","content"]),
        _schema("edit_file","Edit a file in your workspace by exact find/replace (or append:true). Either (a) literal find/replace: give find+replace (find must occur exactly once unless replace_all=true), or (b) append: set append=true and give replace (the text is appended to the end of the file; find ignored). Never rewrite a whole file when a small edit suffices.",{"path":{"type":"string","description":"workspace-relative file path"},"find":{"type":"string","description":"literal text to find (exact, once)"},"replace":{"type":"string","description":"replacement text / appended text"},"append":{"type":"boolean","description":"true = append replace to end of file"},"replace_all":{"type":"boolean","description":"replace every occurrence of find"}},["path","replace"]),
        _schema("read_file","Read a text file (read-only): your workspace files AND, where allowed, the project files (source markdown, class manual, other chapters). Without start_line/end_line the whole file is returned (truncated when large); with start_line/end_line a numbered window of at most 400 lines is returned. Paths are workspace-relative. Mounts: work(rw). Plain paths are relative to work; use \"name:path\" or \"/name/path\" for another mount.",{"path":{"type":"string","description":"file path relative to the workspace root (or name:path for another mount)"},"start_line":{"type":"integer","description":"optional first line (1-based) of a window"},"end_line":{"type":"integer","description":"optional last line (inclusive) of a window"}},["path"]),
        _schema("grep","Search files for a pattern (Go regexp; a pattern that does not compile is matched as plain text). path: a file, or a directory (recursive, text files only). Returns matched lines with 1-based line numbers; use read_file with those lines for context.",{"pattern":{"type":"string","description":"Go regular expression"},"path":{"type":"string","description":"file or directory, workspace-relative; default = workspace root"},"max_matches":{"type":"integer","description":"cap on returned lines (default 50)"}},["pattern"]),
        _schema("compile","Compile your figure code from a workspace file (default figure.tex) and get the compile log plus the output PDF name. Inspect the result with view_pdf {path: standalone.pdf, page: 1}. Write the code with write_file/edit_file first - never paste code into this tool. The file holds the TikZ body (between \\begin{document} and \\end{document}); \\usetikzlibrary / \\usepgfplotslibrary / \\usepackage lines at the top are hoisted into the preamble. There is no preview image: inspect the compiled PDF with view_pdf (crop/zoom re-render it at high resolution), and compare it with the original image via view_image.",{"path":{"type":"string","description":"workspace-relative file holding the TikZ body (default figure.tex)"},"merges":{"type":"array","items":{"type":"string"},"description":"optional: image paths (as in the markdown) of page-boundary continuations absorbed into this figure"}}),
        _schema("view_pdf","Render ONE page of a PDF and attach it as an image: PDFs you compiled (chapter/figure/book) AND, where mounted, the ORIGINAL book PDFs. Crop (percent) and zoom (target pixel width) work the same in both cases; the page is re-rendered from the PDF at the requested width, so zoom stays sharp. Mounts: work(rw). Plain paths are relative to work; use \"name:path\" or \"/name/path\" for another mount.",{"path":{"type":"string","description":"PDF path (workspace-relative, or name:path for another mount)"},"page":{"type":"integer","description":"1-based page number (default 1)"},"left":{"type":"number","description":"crop left percent 0-100"},"top":{"type":"number","description":"crop top percent 0-100"},"right":{"type":"number","description":"crop right percent 0-100 (default 100)"},"bottom":{"type":"number","description":"crop bottom percent 0-100 (default 100)"},"zoom":{"type":"integer","description":"target width in px (default 1280)"}},["path"]),
        _schema("compare_figure","Compare the original image with a compiled figure. Returns a four-panel overlay (gray/R/G/B) or one selected panel; supports targeted crop and zoom for detailed re-checks. This is visual evidence, not semantic proof.",{"pdf_path":{"type":"string","description":"Workspace-relative compiled PDF path; defaults to the latest successful compile."},"detail":{"type":"string","enum":["all","gray","r","g","b","rg","rb","gb"],"description":"all returns a 2x2 gray/R/G/B board; a channel or channel combination returns that panel."},"worst_region":{"type":"boolean","description":"Return only the automatically located weakest comparison region instead of the full figure."},"left":{"type":"number","description":"Crop left percentage, 0-100."},"top":{"type":"number","description":"Crop top percentage, 0-100."},"right":{"type":"number","description":"Crop right percentage, 0-100."},"bottom":{"type":"number","description":"Crop bottom percentage, 0-100."},"zoom":{"type":"integer","description":"Target output width in pixels; 0 keeps the cropped native width."}}),
        _schema("submit","Submit the final TikZ figure. Write the final code to a workspace file (write_file figure.tex) and submit {path: 'figure.tex'} — no need to repeat the code. Allowed only after a successful compile of EXACTLY this code.",{"path":{"type":"string","description":"workspace-relative file holding the final TikZ code (must equal the last successful compile)."},"code":{"type":"string","description":"The final TikZ body code inline (only when not using path)."},"merges":{"type":"string","description":"Image paths (as in the markdown) of page-boundary continuations absorbed into this combined figure."}}),
        _schema("image_context","TEXT context (no pixels): show the markdown lines around an image ref (expandable up/down window, like get_more_context) plus the previous/next image refs with their line numbers. Use it to detect cross-page continuations; use view_image to actually LOOK at an image.",{"image":{"type":"string","description":"Image ref as in the markdown (default: the image this session is drawing). A bare file name is also accepted (resolved inside this document's image folder)."},"up":{"type":"integer","description":"Context lines above (default 10)."},"down":{"type":"integer","description":"Context lines below (default 10)."}}),
        _schema("view_image","LOOK at an image (pixels). path is the image as it appears in the markdown (images/<subject>/foo.jpg) or just its file name (foo.jpg); it is joined to this document's image folder — a wrong name is reported as an error. Compiled PDFs are NOT images: inspect them with view_pdf. Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection. For TEXT context around an image use image_context instead.",{"path":{"type":"string","description":"Image file name (foo.jpg) or the markdown ref (images/<subject>/foo.jpg)."},"left":{"type":"number","description":"Crop left in percent (0-100), default 0."},"top":{"type":"number","description":"Crop top in percent (0-100), default 0."},"right":{"type":"number","description":"Crop right in percent (0-100), default 100."},"bottom":{"type":"number","description":"Crop bottom in percent (0-100), default 100."},"zoom":{"type":"integer","description":"Target width in pixels for the returned image (e.g. 1400 for detail view). Default: fit to 1280."}},["path"])]
    def execute(self,name,args):
        a=_args(args);f=getattr(self,"_tool_"+name,None)
        if f is None:return "Unknown tool: "+name,None
        try:return f(a)
        except Exception as e:return "ERROR: "+str(e),None
    def measure_hint(self):
        m=self.options.get("measurement")
        if isinstance(m,dict) and vector_measure is not None:
            return vector_measure.ImageMeasure(**{k:v for k,v in m.items() if k in vector_measure.ImageMeasure.__dataclass_fields__}).hint()
        if isinstance(m,dict):
            if m.get("found") or m.get("Found"):
                width=m.get('width_mm',m.get('WidthMM',0));height=m.get('height_mm',m.get('HeightMM',0));pw=m.get('pixels_w',m.get('PixelsW',0));ph=m.get('pixels_h',m.get('PixelsH',0));dpi=m.get('dpi',m.get('DPI',0));asp=m.get('aspect_str',m.get('AspectStr',''));page=m.get('page_w_mm',m.get('PageWMM',0));part=f", about {width/page*100:.0f}% of the page width" if page>0 else ""
                return f"ORIGINAL FIGURE SIZE: {width:.1f}mm x {height:.1f}mm on the page{part}; bitmap {pw}x{ph}px, effective resolution {dpi} dpi, aspect {asp}."
            pw=m.get('pixels_w',m.get('PixelsW',0));ph=m.get('pixels_h',m.get('PixelsH',0));asp=m.get('aspect_str',m.get('AspectStr',''))
            return f"ORIGINAL BITMAP: {pw}x{ph}px, aspect {asp}." if asp else ""
        if vector_measure is not None:
            obj=vector_measure.measure_image(self.reference,parse_root=self.options.get("parse_root",""),reference_mm=self.options.get("reference_mm"))
            return obj.hint()
        try:
            with Image.open(self.reference) as im:
                w,h=im.size
            return f"ORIGINAL BITMAP: {w}x{h}px, aspect {w/h:.2f}:1."
        except Exception:return ""

    def rasterize(self,pdf,prefix):
        prefix=Path(prefix);prefix.parent.mkdir(parents=True,exist_ok=True);dpi=int(self.options.get("raster_dpi",110))
        subprocess.run([self.options.get("raster_command","pdftoppm"),"-png","-r",str(dpi),"-singlefile",str(pdf),str(prefix)],check=True,timeout=60,capture_output=True)
        return Path(str(prefix)+".png")
    def _tool_write_file(self,a):
        rel=str(a.get("path",""));c=str(a.get("content",""))
        if not rel:return "REJECTED: path is required.",None
        if Path(rel).suffix.lower() not in {".tex",".cls",".sty",".md"}:return "REJECTED: only .cls .sty .tex .md files are allowed.",None
        p=_safe(self.root,rel);p.parent.mkdir(parents=True,exist_ok=True);p.write_text(c,encoding="utf-8");return f"WROTE {rel} ({len(c.encode())} bytes)",None
    def _tool_edit_file(self,a):
        rel=str(a.get("path",""));p=_safe(self.root,rel);find=str(a.get("find",""));rep=str(a.get("replace",""))
        if not rel:return "ERROR: path 为空",None
        old=p.read_text(encoding="utf-8") if p.exists() else ""
        if a.get("append"):
            p.parent.mkdir(parents=True,exist_ok=True);p.write_text(old+("\n" if old and not old.endswith("\n") else "")+rep,encoding="utf-8");return f"APPENDED {len(rep.encode())} bytes to {rel}.",None
        if not p.exists():return f"NOT FOUND: {rel}（新建文件请用 write_file）",None
        n=old.count(find) if find else 0
        if not find:return "REJECTED: find 为空（追加请设 append=true）",None
        if not n:return f"NOT FOUND: 给出的 find 文本在 {rel} 中没有出现（注意精确匹配，包括缩进）",None
        if n>1 and not a.get("replace_all"):return f"AMBIGUOUS: find 出现 {n} 次；补充上下文让匹配唯一，或设 replace_all=true",None
        before=len(old.encode("utf-8"));new=old.replace(find,rep) if a.get("replace_all") else old.replace(find,rep,1);p.write_text(new,encoding="utf-8");after=len(new.encode("utf-8"));return f"REPLACED {n} occurrence(s) in {rel} ({before} -> {after} bytes).",None
    def _tool_read_file(self,a):
        p=_safe(self.root,str(a.get("path","")));txt=p.read_text(encoding="utf-8")
        if "start_line" in a or "end_line" in a:
            lines=txt.split("\n");start=max(1,int(a.get("start_line",1)));end=min(len(lines),int(a.get("end_line",start)),start+399)
            if end<start:return f"(empty range; file has {len(lines)} lines)",None
            out=[f"[lines {start}-{end} of {len(lines)}]"]
            for i in range(start,end+1):
                line=lines[i-1];out.append(f"{i}: {line[:500]+'...' if len(line)>500 else line}")
            return "\n".join(out)+"\n",None
        data=txt.encode("utf-8");truncated=len(data)>65536
        if truncated:txt=data[:65536].decode("utf-8",errors="ignore")
        count=txt.count("\n")
        if txt and not txt.endswith("\n"):count+=1
        note="\n...[truncated; use start_line/end_line for later parts]..." if truncated else ""
        return f"[{p.name}, {len(data)} bytes, {count} lines]\n{txt}{note}",None
    def _tool_grep(self,a):
        pat=str(a.get("pattern",""));root=_safe(self.root,str(a.get("path",".")));lim=min(200,max(1,int(a.get("max_matches",50))))
        try:
            opts=re2.Options();opts.log_errors=False;rx=re2.compile(pat,options=opts)
        except re2.error:rx=None
        files=[root] if root.is_file() else sorted(p for p in root.rglob("*") if p.is_file() and not ({".git",".trash","node_modules"}&set(p.parts)));out=[];total=0;explicit=str(a.get("path","")).startswith(("work:","/work/"))
        for f in files:
            if f.stat().st_size>8*1024*1024:continue
            try:data=f.read_bytes()
            except OSError:continue
            if b"\\0" in data[:8192]:continue
            try:lines=data.decode("utf-8").split("\n")
            except UnicodeError:continue
            for i,line in enumerate(lines,1):
                if (rx.search(line) if rx else pat in line):
                    total+=1
                    if len(out)<lim:
                        shown=f.relative_to(self.root).as_posix()
                        if explicit:shown="work:"+shown
                        out.append(f"{shown}:{i}:{line}")
        if not total:return "NO MATCHES: "+pat,None
        if total>len(out):out.append(f"...({total-len(out)} more, 收窄 pattern)")
        return "\n".join(out),None
    def _tool_compile(self,a):
        # Source behavior: compile collects merges; submit's string merges is ignored.
        ms=a.get("merges",[])
        if isinstance(ms,list):self.state["merges"].extend(x.strip() for x in ms if isinstance(x,str) and x.strip())
        rel=str(a.get("path","")).strip() or "figure.tex";src=_safe(self.root,rel)
        if not src.is_file():return f"NOT FOUND: {rel}（先用 write_file 把图形代码写入该文件，compile 只接收路径）",None
        code=src.read_text(encoding="utf-8")
        if not code.strip():return f"REJECTED: {rel} 为空。",None
        self.state.update(last_code=code,compile_ok=False,last_pdf="",compare_called=False,best_overlap=0.0,scale_dev_at_best=0.0)
        w=self.root/"standalone.tex";w.write_text(build_standalone(code,any(x in self.engine.lower() for x in ("xe","lua"))),encoding="utf-8");pdf=self.root/"standalone.pdf";had_prev=pdf.is_file()
        try:cp=subprocess.run([self.engine,"-interaction=nonstopmode","-halt-on-error","-no-shell-escape",w.name],cwd=self.root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=int(self.options.get("compile_timeout",120)))
        except Exception as e:self.state["compile_err"]=str(e);return f"COMPILE FAILED. Fix {rel} and call compile again.\nError:\n{e}",None
        if cp.returncode or not pdf.exists():
            err,warn=_compile_summary(cp.stdout);self.state["compile_err"]=err
            text="COMPILE FAILED. Fix "+rel+" and call compile again.\nError:\n"+err
            text+="\n(standalone.pdf still holds the previous build.)" if had_prev else "\n(No standalone.pdf yet.)"
            if warn:text+="\n"+warn[:1500]
            hints=_missing_file_hints(err,self.root)
            if hints:text+="\n"+hints
            return text,None
        self.state.update(compile_ok=True,last_pdf=str(pdf),compile_err="");_,warning_summary=_compile_summary(cp.stdout)
        try:
            info=subprocess.run(["pdfinfo",str(pdf)],capture_output=True,text=True,timeout=30).stdout;pm=re.search(r"(?m)^Pages:\s*(\d+)",info);pages=int(pm.group(1)) if pm else 1;sm=re.search(r"(?m)^Page size:\s*([\d.]+) x ([\d.]+) pts",info);sz=f" Drawn size: {float(sm[1]):.1f}pt x {float(sm[2]):.1f}pt ({float(sm[1])*25.4/72:.1f}mm x {float(sm[2])*25.4/72:.1f}mm, aspect {float(sm[1])/float(sm[2]):.2f}:1)" if sm else ""
        except Exception:pages=1;sz=""
        text=f"COMPILE OK. Artifact: standalone.pdf ({pages} page).{sz} View it with view_pdf {{path: \"standalone.pdf\", page: 1}}."
        if warning_summary:text+="\n"+warning_summary[:1500]
        return text,None
    def _crop(self,img,a):
        l,t,r,b=[_clamp(a.get(k,d)) for k,d in (("left",0),("top",0),("right",100),("bottom",100))]
        if r<=l or b<=t:raise ValueError("crop must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100")
        w,h=img.size;return img.crop((round(l*w/100),round(t*h/100),max(1,round(r*w/100)),max(1,round(b*h/100))))
    def _tool_view_pdf(self,a):
        pdf=_safe(self.root,str(a.get("path","")))
        if pdf.suffix.lower()!=".pdf":return "REJECTED: view_pdf 只接受 .pdf（图片请用 view_image）",None
        page=max(1,int(a.get("page",1)));zoom=min(6000,max(1,int(a.get("zoom",0) or a.get("zoom_width",0) or 1280)));self._pdf_views[(str(pdf),page)]=self._pdf_views.get((str(pdf),page),0)+1;info=self._pdf_info(pdf,page)
        if info[0] and page>info[0]:return f"OUT OF RANGE: {pdf.name} 共 {info[0]} 页，请求第 {page} 页",None
        left,top,right,bottom=[_clamp(a.get(k,d)) for k,d in (("left",0),("top",0),("right",100),("bottom",100))]
        frac=(right-left)/100
        if frac<.02:frac=1
        pagew=min(6000,max(400,int(zoom/frac)+8));img=self._pdf_render(pdf,page,pagew);img=self._crop(img,{"left":left,"top":top,"right":right,"bottom":bottom})
        if img.width!=zoom:img=_catmull_resize(img,zoom)
        out=self.root/f"view_pdf_{page}.jpg";img.save(out,quality=90)
        wmm,hmm=info[1],info[2];size_note=f", page {wmm:.1f}mm x {hmm:.1f}mm" if wmm>0 and hmm>0 else ""
        if wmm>0 and hmm>0 and (left>0 or top>0 or right<100 or bottom<100):
            fw=(right-left)/100;fh=(bottom-top)/100
            if fw>0 and fh>0:size_note+=f", this crop {wmm*fw:.1f}mm x {hmm*fh:.1f}mm"
        budget=self._budget_note("view_pdf",str(pdf)+f"#{page}",self._pdf_views[(str(pdf),page)],int(self.options.get("view_pdf_max",0)))
        return f"PDF page {pdf.name} p{page} (crop {left:.0f}%,{top:.0f}%-{right:.0f}%,{bottom:.0f}%, width {zoom}px{size_note} (page {page}/{info[0]}){budget}) attached.",str(out)
    def _pdf_info(self,pdf,page=1):
        try:
            out=subprocess.run(["pdfinfo","-f",str(page),"-l",str(page),str(pdf)],capture_output=True,text=True,timeout=30,check=True).stdout
            n=re.search(r"(?m)^Pages:\s*(\d+)",out);s=re.search(r"(?m)^Page(?:\s+\d+)? size:\s*([\d.]+) x ([\d.]+) pts",out)
            return (int(n.group(1)) if n else 0,float(s.group(1))*25.4/72 if s else 0,float(s.group(2))*25.4/72 if s else 0)
        except Exception:return (0,0,0)
    def _pdf_render(self,pdf,page,width):
        td=Path(tempfile.mkdtemp(prefix="vector-view-"));prefix=td/"p"
        try:
            subprocess.run([self.options.get("raster_command","pdftoppm"),"-png","-f",str(page),"-l",str(page),"-scale-to-x",str(width),"-scale-to-y","-1","-singlefile",str(pdf),str(prefix)],check=True,timeout=120,capture_output=True);return Image.open(Path(str(prefix)+".png")).convert("RGB").copy()
        finally:shutil.rmtree(td,ignore_errors=True)
    def _budget_note(self,kind,key,used,maximum):
        if maximum<=0:return ""
        ratio=float(self.options.get("warn_ratio",.7));warn=max(1,__import__("math").ceil(maximum*ratio));label=f"{kind} on {Path(key.split('#')[0]).name}"
        if used>maximum:return f" VIEW BUDGET SPENT ({label}: {used}/{maximum})."
        if used>=warn and key not in self._warned_budget:
            self._warned_budget.add(key);return f" VIEW BUDGET: {label} used {used}/{maximum}, {maximum-used} left."
        return ""
    def _tool_view_image(self,a):
        rel=str(a.get("path",""));p=Path(rel)
        if p.is_absolute():raise ValueError(f"只允许相对路径: {rel}")
        sub=rel.replace("\\\\","/").removeprefix("./").removeprefix("images/");cur=str(self.current_img).replace("\\\\","/").removeprefix("./").removeprefix("images/");subject=Path(cur).parent;cands=[self.images_dir/sub]
        if str(subject)!=".":cands.insert(0,self.images_dir/subject/sub if not str(sub).startswith(str(subject)+"/") else self.images_dir/sub)
        resolved=[x.resolve() for x in cands if x.resolve().is_relative_to(self.images_dir) and x.is_file()]
        if not resolved and "/" not in sub:
            hits=list(self.images_dir.rglob(sub))
            if len(hits)>1:raise ValueError("文件名不唯一，请给完整路径: "+", ".join(str(x.relative_to(self.images_dir)) for x in hits))
            resolved=hits
        p=resolved[0] if resolved else None
        if p is None:raise FileNotFoundError(f"文件不存在: {rel}（{self.images_dir}）")
        self._views[str(p)]=self._views.get(str(p),0)+1
        with Image.open(p) as source:img=Image.alpha_composite(Image.new("RGBA",source.size,"white"),source.convert("RGBA")).convert("RGB")
        img=self._crop(img,a);z=min(6000,max(1,int(a.get("zoom",1280))))
        if img.width!=z:img=_catmull_resize(img,z)
        out=self.root/"view_image.jpg";img.save(out,quality=90)
        measurement=vector_measure.measure_image(p,parse_root=self.options.get("parse_root","")) if vector_measure is not None else None
        note=" "+measurement.hint() if measurement is not None and measurement.hint() else ""
        if measurement is not None and any(float(a.get(k,d))!=d for k,d in (("left",0),("top",0),("right",100),("bottom",100))):
            crop=measurement.crop(*[float(a.get(k,d)) for k,d in (("left",0),("top",0),("right",100),("bottom",100))]);h=crop.crop_hint()
            if h:note+=" "+h+"."
        note+=self._budget_note("view_image",str(p),self._views[str(p)],int(self.options.get("view_image_max",0)))
        return f"Image {rel} (crop attached)."+note,str(out)
    def _tool_image_context(self,a):
        content=str(self.options.get("md_content",""));lines=content.splitlines();refs=[]
        for i,line in enumerate(lines):refs.extend((m.group(1),i) for m in re.finditer(r"(?i)(?:!\[.*?\]\(|<img[^>]*?src=[\"'])(images/.+?\.(?:jpg|jpeg|png|gif|webp))(?:\)|[\"'][^>]*>)",line))
        target=str(a.get("image") or self.current_img);exact=next((i for i,(p,_) in enumerate(refs) if p==target),-1)
        idx=exact
        if idx<0:
            norm=target.replace("\\\\","/").removeprefix("./").removeprefix("images/")
            if "/" in norm:
                hits=[i for i,(p,_) in enumerate(refs) if p.removeprefix("images/")==norm or p.endswith("/"+norm)]
            else:hits=[i for i,(p,_) in enumerate(refs) if Path(p).name==norm]
            idx=hits[0] if len(hits)==1 else -1
        if idx<0:return f"markdown 中找不到图片 {target}",None
        up=10;down=10
        if "up" in a and int(a["up"])>0:up=int(a["up"])
        if "down" in a and int(a["down"])>0:down=int(a["down"])
        if self.options.get("max_up",0)>0:up=min(up,int(self.options["max_up"]))
        if self.options.get("max_down",0)>0:down=min(down,int(self.options["max_down"]))
        def block(i):
            if i<0 or i>=len(refs):return "(none)"
            path,line=refs[i];delta=line-refs[idx][1];suffix=f", {delta:+d} lines from this image" if delta else "";return f"{path} (line {line+1}{suffix})\n"+"\n".join(("> " if n==line else "  ")+f"{n+1:4d} | {lines[n]}" for n in range(max(0,line-up),min(len(lines),line+down+1)))
        return f"image {idx+1} of {len(refs)} in this document: {refs[idx][0]}\n\n## PREVIOUS image ref:\n{block(idx-1)}\n\n## THIS image context (up {up} / down {down} lines; request again with bigger up/down to expand):\n{block(idx)}\n\n## NEXT image ref:\n{block(idx+1)}",None
    def _tool_compare_figure(self,a):
        explicit=str(a.get("pdf_path","")).strip()
        if not explicit and (not self.state["compile_ok"] or not self.state["last_pdf"]):return "Comparison unavailable: compile a figure successfully first, or provide pdf_path.",None
        pdf=_safe(self.root,explicit) if explicit else Path(self.state["last_pdf"])
        if not pdf.is_file():return "Comparison unavailable: PDF not found.",None
        with Image.open(self.reference) as source:ref=source.copy()
        with tempfile.TemporaryDirectory(prefix="vector-candidate-") as td:
            cand_path=self.rasterize(pdf,Path(td)/"candidate");cand=Image.open(cand_path).convert("RGB")
            refmm=tuple(self.options.get("reference_mm",(0,0)));measurement=self.options.get("measurement") or {}
            if not any(refmm) and isinstance(measurement,dict):refmm=(measurement.get("width_mm",measurement.get("WidthMM",0)),measurement.get("height_mm",measurement.get("HeightMM",0)))
            _,cw,ch=self._pdf_info(pdf,1);canvas=(float(refmm[0]),float(refmm[1]),cw,ch) if len(refmm)>=2 else (0,0,cw,ch)
            m=vector_metrics.compare_images(ref,cand,canvas_mm=canvas)
            detail=str(a.get("detail","all")).lower()
            if detail not in {"all","gray","r","g","b","rg","rb","gb"}:return "Comparison unavailable: detail must be all, gray, r, g, b, rg, rb, or gb.",None
            l,t,r,b=[float(a.get(k,d)) for k,d in (("left",0),("top",0),("right",100),("bottom",100))]
            if l<0 or t<0 or r>100 or b>100 or r<=l or b<=t:return "Comparison unavailable: crop must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100.",None
            if a.get("worst_region") and "left" not in a:l,t,r,b=vector_metrics.worst_region(ref,cand)
            img=vector_metrics.make_overlay(ref,cand,int(m["dx"]),int(m["dy"]),detail);img=self._crop(img,{"left":l,"top":t,"right":r,"bottom":b});z=min(6000,max(0,int(a.get("zoom",0))))
            if z and img.width!=z:img=_catmull_resize(img,z)
            out=self.root/"compare_figure.png";img.save(out)
        self.state["compare_called"]=True;self._last_metrics=m
        if not self.state["best_overlap"] or m["similarity"]>self.state["best_overlap"]:
            mp={"similarity":"best_overlap","scale_dev":"scale_dev_at_best","scale_x":"best_scale_x","scale_y":"best_scale_y","size_unit":"size_unit_at_best","aspect_ref":"aspect_ref_at_best","aspect_cand":"aspect_cand_at_best","ink_ref_w":"ink_ref_w_at_best","ink_ref_h":"ink_ref_h_at_best","ink_cand_w":"ink_cand_w_at_best","ink_cand_h":"ink_cand_h_at_best"}
            for x,y in mp.items():self.state[y]=m.get(x,"" if x=="size_unit" else 0)
        c=m.get("channels",[0,0,0]);runs="; ".join(f"{r.get('name','')} {r.get('score',0)*100:.2f}% @({r.get('dx',0)},{r.get('dy',0)})" for r in m.get("runs",[]))
        if m.get("ink_ref_w",0)<=0 or m.get("ink_cand_w",0)<=0:size_sentence="Ink size: not measurable (no ink in one of the images)."
        elif m.get("size_unit")=="mm":size_sentence=(f"Ink size: canvas share w {m.get('share_ref_w',0)*100:.0f}%/{m.get('share_cand_w',0)*100:.0f}%, h {m.get('share_ref_h',0)*100:.0f}%/{m.get('share_cand_h',0)*100:.0f}% (original/candidate); physical ink {m['ink_ref_w']:.1f}mm x {m['ink_ref_h']:.1f}mm vs {m['ink_cand_w']:.1f}mm x {m['ink_cand_h']:.1f}mm; width x{m['scale_x']:.3f}, height x{m['scale_y']:.3f}, max deviation {m['scale_dev']*100:.1f}% (measured in mm BEFORE the comparison rescales the candidate, so it does catch a wrong printed size; fix the coordinates/canvas rather than the stroke width). Ink aspect ratio {m['aspect_ref']:.2f}:1 vs {m['aspect_cand']:.2f}:1.")
        else:size_sentence=(f"Ink size: canvas share w {m.get('share_ref_w',0)*100:.0f}%/{m.get('share_cand_w',0)*100:.0f}%, h {m.get('share_ref_h',0)*100:.0f}%/{m.get('share_cand_h',0)*100:.0f}% (original/candidate); physical size unknown (image or PDF page size not measurable), so only the aspect ratio is checked: {m['aspect_ref']:.2f}:1 vs {m['aspect_cand']:.2f}:1, deviation {m['scale_dev']*100:.1f}% >? limit. Ink bbox {int(m['ink_ref_w'])}x{int(m['ink_ref_h'])}px vs {int(m['ink_cand_w'])}x{int(m['ink_cand_h'])}px is NOT comparable (render DPI differs) — do not aim at it. Fix the coordinates/canvas rather than the stroke width.")
        return f"Soft-ink overlap: {m['similarity']*100:.2f}% (R {c[0]*100:.2f}%, G {c[1]*100:.2f}%, B {c[2]*100:.2f}%, gray {m.get('gray',0)*100:.2f}%). {size_sentence} Estimated candidate translation applied: dx={m['dx']} px, dy={m['dy']} px. Alignment starts: {runs}. Detail={detail}, crop={l:.1f}%,{t:.1f}%-{r:.1f}%,{b:.1f}%, output={img.width}x{img.height}px. Overlay key: white=shared ink, magenta=original-only ink, green=candidate-only ink, black=background.",str(out)
    def _tool_submit(self,a):
        live=self.options.get("live_threshold");th=self.min_overlap
        if live is not None:
            value=live() if callable(live) else live
            if value is not None and float(value)!=0:self.min_overlap=float(value)
        th=self.min_overlap
        code=str(a.get("code",""));rel=str(a.get("path","" )).strip()
        if rel:
            p=_safe(self.root,rel)
            if not p.is_file():return f"NOT FOUND: {rel}（先 write_file 写入最终代码）",None
            code=p.read_text(encoding="utf-8")
        if not code.strip():return "ERROR: code 为空（或 path 文件不存在）",None
        self.state["submit_attempts"]+=1
        if not self.state["compare_called"]:return "REJECTED: call compare_figure at least once after a successful compile, inspect its comparison, and then submit.",None
        ov=self.state["best_overlap"];dev=self.state["scale_dev_at_best"];n=self.accept_min_submits;accept=bool(a.get("accept_as_is",False));note=str(a.get("note",""));why=[]
        if th>0 and ov<th:why.append(f"软墨重叠 {ov*100:.2f}% < 门槛 {th*100:.2f}%（差 {(th-ov)*100:.2f} 个百分点）")
        lim=self.max_scale_dev
        if lim>0 and dev>lim:
            if self.state["size_unit_at_best"]=="mm":why.append(f"墨迹尺寸偏差 {dev*100:.1f}% > 上限 {lim*100:.1f}%（宽 {self.state['ink_ref_w_at_best']:.1f}mm → {self.state['ink_cand_w_at_best']:.1f}mm = 原图的 {self.state['best_scale_x']:.2f} 倍；高 {self.state['ink_ref_h_at_best']:.1f}mm → {self.state['ink_cand_h_at_best']:.1f}mm = {self.state['best_scale_y']:.2f} 倍）")
            elif self.state["size_unit_at_best"]=="aspect":why.append(f"墨迹宽高比偏差 {dev*100:.1f}% > 上限 {lim*100:.1f}%（原图 {self.state['aspect_ref_at_best']:.2f}:1，你画的是 {self.state['aspect_cand_at_best']:.2f}:1；物理尺寸查不到，只能比形状）")
            else:why.append(f"宽高尺寸偏差 {dev*100:.1f}% > 上限 {lim*100:.1f}%（宽的 {self.state['best_scale_x']:.2f} 倍、高的 {self.state['best_scale_y']:.2f} 倍）")
        if accept:
            if n>0 and self.state["submit_attempts"]<n:return "REJECTED: "+("；".join(why) or "对照原图看差在哪（线条、比例、标注位置）")+f"。这张图目前只交过 {self.state['submit_attempts']-1} 次 submit，先把图改好再交。",None
            if not note.strip():return "REJECTED: accept_as_is needs a note saying what could not be matched and why it is as good as it gets.",None
        elif why:
            msg="REJECTED: "+"；".join(why)+"。先改图再 compile + compare_figure 重新提交。"
            if n<=0 or self.state["submit_attempts"]>=n:msg+=f"这张图已经交过 {self.state['submit_attempts']} 次：如果确实只能做到这儿，可以 submit {{accept_as_is:true, note:\"差在哪里、为什么到不了\"}} 交这一版（记为尽力还原，之后不会自动重画）。"
            return msg,None
        if not self.state["compile_ok"] or code.strip()!=self.state.get("last_code","").strip():return "REJECTED: the submitted code differs from the last successful compile. Call compile {path} with this exact file first.",None
        self.state.update(submitted=True,final_code=code,uncertain="[?]" in code,accepted_as_is=accept,accept_note=note.strip() if accept else "")
        return (f"SUBMITTED as best-effort (overlap {ov*100:.2f}%, size deviation {dev*100:.1f}%): recorded so this figure is not redrawn automatically. {note.strip()}" if accept else f"SUBMITTED. The figure is accepted (overlap {ov*100:.2f}%, size deviation {dev*100:.1f}%)."),None
    def deliver(self):
        if not self.state["submitted"]:raise RuntimeError("figure has not been submitted")
        pdf=self.root/"submitted.pdf";body=self.root/"submitted.body.tex";tex=self.root/"submitted.tex";preview_dir=self.root/"预览";preview_dir.mkdir(parents=True,exist_ok=True);preview=preview_dir/"submitted.png"
        source_pdf=Path(self.state.get("last_pdf", ""))
        if not source_pdf.is_file():
            wrapper=self.root/"standalone.tex";wrapper.write_text(build_standalone(self.state["final_code"],any(x in self.engine.lower() for x in ("xe","lua"))),encoding="utf-8")
            cp=subprocess.run([self.engine,"-interaction=nonstopmode","-halt-on-error","-no-shell-escape",wrapper.name],cwd=self.root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=int(self.options.get("compile_timeout",120)))
            source_pdf=self.root/"standalone.pdf"
            if cp.returncode or not source_pdf.is_file():raise RuntimeError("交付前重新编译失败："+cp.stdout[-3000:])
            self.state["last_pdf"]=str(source_pdf)
        body.write_text(self.state["final_code"],encoding="utf-8");tex.write_text(build_standalone(self.state["final_code"],any(x in self.engine.lower() for x in ("xe","lua"))),encoding="utf-8")
        shutil.copy2(source_pdf,pdf)
        preview_warning=""
        try:
            rendered=self.rasterize(pdf,preview.with_suffix(""))
            if rendered!=preview:rendered.replace(preview)
        except Exception as exc:preview_warning=f"TikZ最终栅格化失败（PDF已保留）: {exc}"
        result={"submitted":True,"submitted_tex":str(tex),"submitted_body_tex":str(body),"submitted_pdf":str(pdf),"preview":str(preview) if preview.exists() else None,"merges":self.state["merges"],"uncertain":self.state["uncertain"],"best_overlap":self.state["best_overlap"],"scale_dev":self.state["scale_dev_at_best"],"accepted_as_is":self.state["accepted_as_is"],"accept_note":self.state["accept_note"],"submit_attempts":self.state["submit_attempts"]}
        if preview_warning:result["warning"]=preview_warning
        tmp=self.root/"result.json.tmp";tmp.write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8");tmp.replace(self.root/"result.json")
        return result
