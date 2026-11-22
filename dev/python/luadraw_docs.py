"""Cached LuaDraw docs/source search and PDF crop tool."""
from __future__ import annotations
import argparse, base64, hashlib, json, os, re, shutil, subprocess, sys, tempfile, time
from html.parser import HTMLParser
from pathlib import Path

SCHEMA_VERSION = 2
PROJECT_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_ROOT = PROJECT_ROOT / "vendor/luadraw-3.7/luadraw"


def _schema():
    return {"type": "function", "function": {"name": "luadraw_doc", "description": "Search/read/view LuaDraw docs. Search hits report PDF bbox in points; left/top/right/bottom crop inputs are page percentages.", "parameters": {"type": "object", "properties": {
        "mode": {"type": "string", "enum": ["search", "read", "view"]}, "query": {"type": "string"}, "doc": {"type": "string"},
        "page": {"type": "integer"}, "left": {"type": "number"}, "top": {"type": "number"}, "right": {"type": "number"}, "bottom": {"type": "number"},
        "zoom": {"type": "integer"}, "limit": {"type": "integer"}, "offset": {"type": "integer"}, "start_line": {"type": "integer"}, "end_line": {"type": "integer"},
        "include_image": {"type": "boolean", "description": "Attach one relevant PDF crop when available"}}, "required": []}}}


def _clamp(value, default):
    try: return max(0.0, min(100.0, float(value)))
    except (TypeError, ValueError): return default


class _BBoxParser(HTMLParser):
    """Read Poppler pdftotext -bbox word coordinates without third-party XML libs."""
    def __init__(self):
        super().__init__(); self.pages=[]; self.page=None; self.word_box=None; self.word_text=""
    def handle_starttag(self, tag, attrs):
        attrs=dict(attrs)
        if tag=="page":
            self.page={"width":float(attrs["width"]),"height":float(attrs["height"]),"words":[]}
        elif tag=="word" and self.page is not None:
            self.word_box=tuple(float(attrs[k]) for k in ("xmin","ymin","xmax","ymax")); self.word_text=""
    def handle_data(self,data):
        if self.word_box is not None:self.word_text+=data
    def handle_endtag(self,tag):
        if tag=="word" and self.word_box is not None:
            self.page["words"].append((self.word_text,*self.word_box)); self.word_box=None
        elif tag=="page" and self.page is not None:
            self.pages.append(self.page); self.page=None


class LuaDrawDocs:
    """Cached documentation index with source-window and PDF-crop operations."""
    def __init__(self, root=None, cache_dir=None):
        self.root = Path(root or DEFAULT_ROOT).expanduser().resolve()
        if not self.root.is_dir(): raise FileNotFoundError(f"LuaDraw root not found: {self.root}")
        self.cache_dir = Path(cache_dir).expanduser().resolve() if cache_dir else PROJECT_ROOT / "runs/cache/doc-index"
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self.cache_path = self.cache_dir / "luadraw-doc-index-v2.json"
        self._index = None
        try:
            import fitz
            fitz.TOOLS.mupdf_display_errors(False)
            fitz.TOOLS.mupdf_display_warnings(False)
            self._fitz = fitz
        except ImportError:
            self._fitz = None
            if not shutil.which("pdftotext") or not shutil.which("pdftoppm"):
                raise RuntimeError("PDF rendering/indexing requires PyMuPDF, or Poppler pdftotext and pdftoppm")

    @staticmethod
    def definition(): return _schema()

    def _files(self):
        files=[]
        for folder,patterns in (("doc/src",("*.tex",)),("examples",("*.tex","*.lua")),("files",("*.lua",))):
            base=self.root/folder
            if base.is_dir():
                for pattern in patterns: files.extend(p for p in base.rglob(pattern) if p.is_file())
        docdir=self.root/"doc"
        if docdir.is_dir(): files.extend(p for p in docdir.glob("*.pdf") if p.is_file())
        return sorted(set(files))

    def _version(self):
        for path in (self.root/"doc/src/luadraw-doc-en.tex",self.root/"doc/src/luadraw-doc-fr.tex"):
            if path.is_file():
                match=re.search(r"\\def\\version\{([^}]+)\}",path.read_text(encoding="utf-8",errors="replace"))
                if match:return match.group(1)
        readme=self.root/"README.md"
        if readme.is_file():
            match=re.search(r"Package Version:\s*\*\*([^*]+)\*\*",readme.read_text(encoding="utf-8",errors="replace"))
            if match:return match.group(1).strip()
        return "unknown"

    def _pdf_pages(self,path):
        if self._fitz is None:
            cp=subprocess.run(["pdftotext","-bbox",str(path),"-"],capture_output=True,text=True,check=True)
            parser=_BBoxParser();parser.feed(cp.stdout);pages=[]
            for number,page in enumerate(parser.pages,1):
                lines=[]
                for word in page["words"]:
                    y=(word[2]+word[4])/2
                    if not lines or abs(lines[-1][0][2]-y)>3:lines.append([])
                    lines[-1].append(word)
                blocks=[]
                for words in lines:
                    blocks.append({"text":" ".join(w[0] for w in words),"bbox":[min(w[1] for w in words),min(w[2] for w in words),max(w[3] for w in words),max(w[4] for w in words)]})
                pages.append({"page":number,"width":page["width"],"height":page["height"],"blocks":blocks})
            return pages
        pages=[]
        with self._fitz.open(path) as pdf:
            for number,page in enumerate(pdf,1):
                blocks=[]
                for block in page.get_text("dict")["blocks"]:
                    if "lines" not in block:continue
                    text="\n".join("".join(span["text"] for span in line["spans"]) for line in block["lines"]).strip()
                    if text:blocks.append({"text":text,"bbox":list(block["bbox"])})
                pages.append({"page":number,"width":page.rect.width,"height":page.rect.height,"blocks":blocks})
        return pages

    def _build(self,files,fingerprint):
        docs=[];digest=hashlib.sha256()
        for path,meta in zip(files,fingerprint):
            digest.update(meta["path"].encode());digest.update(str(meta["size"]).encode());digest.update(str(meta["mtime_ns"]).encode());digest.update(path.read_bytes())
            if path.suffix.lower()==".pdf":docs.append({"path":meta["path"],"kind":"pdf","pages":self._pdf_pages(path)})
            else:
                text=path.read_text(encoding="utf-8",errors="replace")
                docs.append({"path":meta["path"],"kind":"source","text":text})
        return {"schema":SCHEMA_VERSION,"root":str(self.root),"version":self._version(),"created":time.time(),"key":digest.hexdigest(),"fingerprint":fingerprint,"docs":docs}

    def _get_index(self):
        files=self._files();fingerprint=[{"path":p.relative_to(self.root).as_posix(),"size":p.stat().st_size,"mtime_ns":p.stat().st_mtime_ns} for p in files]
        if self._index is not None and self._index.get("fingerprint")==fingerprint:return self._index
        try:
            cached=json.loads(self.cache_path.read_text(encoding="utf-8"))
            if cached.get("schema")==SCHEMA_VERSION and cached.get("root")==str(self.root) and cached.get("fingerprint")==fingerprint:
                self._index=cached;return cached
        except (OSError,ValueError):pass
        self._index=self._build(files,fingerprint)
        with tempfile.NamedTemporaryFile(mode="w",encoding="utf-8",dir=self.cache_dir,prefix="luadraw-index-",suffix=".tmp",delete=False) as handle:
            json.dump(self._index,handle,ensure_ascii=False,separators=(",",":"));tmp=Path(handle.name)
        os.replace(tmp,self.cache_path)
        return self._index

    def _safe_doc(self,name,kind=None):
        idx=self._get_index();path=(self.root/name).resolve()
        if not path.is_relative_to(self.root):raise ValueError("document path must stay inside LuaDraw root")
        rel=path.relative_to(self.root).as_posix()
        for doc in idx["docs"]:
            if doc["path"]==rel and (kind is None or doc["kind"]==kind):return doc
        raise FileNotFoundError(f"document not indexed under root: {name}")

    def _render(self,pdf,page_no,bbox,args,out_dir,page_info=None):
        out_dir.mkdir(parents=True,exist_ok=True);target=out_dir/f"luadraw-p{page_no}-{time.time_ns()}.png"
        left,top,right,bottom=(_clamp(args.get(k),d) for k,d in (("left",0),("top",0),("right",100),("bottom",100)))
        if bbox is not None and not any(k in args for k in ("left","top","right","bottom")):
            width,height=page_info["width"],page_info["height"]
            crop_w=min(width,max(bbox[2]-bbox[0]+width*.08,width*.82)); crop_h=min(height,max(bbox[3]-bbox[1]+height*.04,height*.42))
            cx,cy=(bbox[0]+bbox[2])/2,(bbox[1]+bbox[3])/2
            left=max(0,(cx-crop_w/2)/width*100);right=min(100,(cx+crop_w/2)/width*100)
            top=max(0,(cy-crop_h/2)/height*100);bottom=min(100,(cy+crop_h/2)/height*100)
        zoom=min(5000,max(200,int(args.get("zoom",1200) or 1200)))
        if self._fitz:
            with self._fitz.open(pdf) as doc:
                page=doc[page_no-1];rect=page.rect
                clip=self._fitz.Rect(rect.x0+rect.width*left/100,rect.y0+rect.height*top/100,rect.x0+rect.width*right/100,rect.y0+rect.height*bottom/100)
                page.get_pixmap(matrix=self._fitz.Matrix(zoom/max(1,clip.width),zoom/max(1,clip.width)),clip=clip,alpha=False,annots=False).save(target)
        else:
            from PIL import Image
            crop_fraction=max(.02,(right-left)/100);whole_width=min(6000,max(400,int(zoom/crop_fraction)+2))
            prefix=target.with_suffix("");subprocess.run(["pdftoppm","-f",str(page_no),"-l",str(page_no),"-scale-to-x",str(whole_width),"-scale-to-y","-1","-png",str(pdf),str(prefix)],check=True,capture_output=True)
            image=Path(f"{prefix}-{page_no}.png")
            if not image.exists():image=Path(f"{prefix}-1.png")
            with Image.open(image) as im:
                cropped=im.crop((round(im.width*left/100),round(im.height*top/100),round(im.width*right/100),round(im.height*bottom/100)))
                height=max(1,round(cropped.height*zoom/max(1,cropped.width)))
                cropped.resize((zoom,height),Image.Resampling.LANCZOS).save(target)
            image.unlink()
        return str(target)

    def _search(self,query,limit,offset):
        terms=[t.lower() for t in re.findall(r"[\w]+|[^\W\d_]+",query,re.UNICODE) if len(t)>1] or [query.lower()];hits=[]
        for doc in self._get_index()["docs"]:
            if doc["kind"]=="source":
                for n,line in enumerate(doc["text"].splitlines(),1):
                    score=sum(line.lower().count(t) for t in terms)
                    if score:hits.append((score,{"doc":doc["path"],"kind":"source","line":n,"text":line.strip()[:400]}))
            else:
                for pg in doc["pages"]:
                    for block in pg["blocks"]:
                        score=sum(block["text"].lower().count(t) for t in terms)
                        if score:
                            # Manual contents pages repeat symbols without showing the API itself.
                            rank=score+(2 if pg["page"]>10 else -3)+(2 if len(block["text"].splitlines())>1 else 0)
                            hits.append((rank,{"doc":doc["path"],"kind":"pdf","page":pg["page"],"bbox":block["bbox"],"text":block["text"][:600],"page_info":pg}))
        hits.sort(key=lambda item:(-item[0],item[1]["doc"],item[1].get("page",0),item[1].get("line",0)))
        return [hit for _,hit in hits[offset:offset+limit]]

    def execute(self,args,out_dir):
        if isinstance(args,str):args=json.loads(args or "{}")
        if not isinstance(args,dict):raise ValueError("tool arguments must be a JSON object")
        out_dir=Path(out_dir).resolve();mode=args.get("mode","search");idx=self._get_index()
        meta=f"LuaDraw {idx['version']} | index_sha256 {idx['key']} | schema {idx['schema']} | source_mtime_ns {max((item['mtime_ns'] for item in idx['fingerprint']),default=0)} | indexed {time.strftime('%Y-%m-%d %H:%M:%S',time.localtime(idx['created']))}"
        if mode=="search":
            query=str(args.get("query","")).strip()
            if not query:raise ValueError("search requires query")
            hits=self._search(query,min(20,max(1,int(args.get("limit",5)))),max(0,int(args.get("offset",0))));rows=[];image=None
            for i,hit in enumerate(hits,1):
                suffix=f" p.{hit['page']} bbox={','.join(f'{v:.1f}' for v in hit['bbox'])}" if hit["kind"]=="pdf" else f" line {hit['line']}"
                rows.append(f"{i}. {hit['doc']}{suffix}\n   {hit['text']}")
                if image is None and hit["kind"]=="pdf" and args.get("include_image",True):image=self._render(self.root/hit["doc"],hit["page"],hit["bbox"],args,out_dir,hit["page_info"])
            return meta+"\n"+("\n".join(rows) if rows else "No matches."),image
        if mode=="read":
            doc=self._safe_doc(str(args.get("doc",args.get("query",""))),"source");lines=doc["text"].splitlines()
            start=max(1,int(args.get("start_line",1)));end=min(len(lines),int(args.get("end_line",start+79)),start+399)
            body="\n".join(f"{n}: {lines[n-1]}" for n in range(start,end+1))
            return f"{meta}\n{doc['path']} lines {start}-{end}/{len(lines)}\n{body}",None
        if mode=="view":
            doc=self._safe_doc(str(args.get("doc","")),"pdf");page_no=int(args.get("page",1));pages=doc["pages"]
            if page_no<1 or page_no>len(pages):raise ValueError(f"page out of range: {page_no}; document has {len(pages)} pages")
            image=self._render(self.root/doc["path"],page_no,None,args,out_dir,pages[page_no-1])
            return f"{meta}\nPDF {doc['path']} page {page_no}/{len(pages)} attached.",image
        raise ValueError("mode must be search, read, or view")


def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument("--root");parser.add_argument("--cache-dir");parser.add_argument("--request");parser.add_argument("--jsonl",action="store_true")
    ns=parser.parse_args(argv);tool=LuaDrawDocs(ns.root,ns.cache_dir)
    def run(line):
        request=json.loads(line);out_dir=request.pop("out_dir",tempfile.gettempdir());inline=request.pop("inline_image",False)
        text,image=tool.execute(request,out_dir);result={"text":text,"image_path":image}
        if image and inline:result["image_base64"]=base64.b64encode(Path(image).read_bytes()).decode("ascii")
        return json.dumps(result,ensure_ascii=False)
    if ns.jsonl:
        for line in sys.stdin:
            try:print(run(line),flush=True)
            except Exception as exc:print(json.dumps({"error":str(exc)},ensure_ascii=False),flush=True)
        return 0
    raw=Path(ns.request).read_text(encoding="utf-8") if ns.request else sys.stdin.read()
    try:print(run(raw));return 0
    except Exception as exc:print(json.dumps({"error":str(exc)},ensure_ascii=False));return 1

if __name__=="__main__":raise SystemExit(main())
