"""Python port of DocVision figure comparison metrics (HEAD 20faca6).

Candidate resizing ports x/image/draw v0.39.0 Catmull-Rom separable scaling;
image decoder color conversion can still differ from Go at format boundaries.
"""
from __future__ import annotations

import math
import numpy as np
from PIL import Image


def _round(v):
    return math.floor(v + 0.5) if v >= 0 else math.ceil(v - 0.5)


def _rgb(image):
    rgba = np.asarray(image.convert("RGBA"), dtype=np.uint16)
    a = rgba[:, :, 3:4]
    return ((rgba[:, :, :3] * a + 255 * (255 - a)) // 255).astype(np.uint8)


def _catmull_rom(t):
    if t < 1:
        return ((1.5*t - 2.5)*t*t) + 1
    return ((((-0.5*t + 2.5)*t)-4)*t) + 2


def _axis_weights(src_size, dst_size):
    scale = src_size / dst_size
    half = 2.0 * scale if scale > 1 else 2.0
    arg_scale = 1.0 / scale if scale > 1 else 1.0
    result = []
    for x in range(dst_size):
        center = (x + 0.5)*scale - 0.5
        lo = max(0, math.floor(center-half))
        hi = min(src_size, math.ceil(center+half))
        weights = []
        total = 0.0
        for coord in range(lo, hi):
            t = abs((center-coord)*arg_scale)
            weight = _catmull_rom(t) if t < 2 else 0.0
            if weight != 0:
                weights.append((coord, weight))
                total += weight
        result.append((weights, total))
    return result


def _catmull_rom_resize(rgb, width, height):
    src_h, src_w = rgb.shape[:2]
    if (src_w, src_h) == (width, height):
        return rgb
    xweights = _axis_weights(src_w, width)
    tmp = np.zeros((src_h, width, 3), dtype=np.float64)
    # Go accumulates 16-bit premultiplied channels, then applies the reciprocal.
    src16 = rgb.astype(np.float64) * 257.0
    for x, (weights, total) in enumerate(xweights):
        for sx, weight in weights:
            tmp[:, x, :] += src16[:, sx, :] * weight
        tmp[:, x, :] *= (1.0 / total) / 65535.0
    yweights = _axis_weights(src_h, height)
    out16 = np.zeros((height, width, 3), dtype=np.float64)
    for y, (weights, total) in enumerate(yweights):
        for sy, weight in weights:
            out16[y, :, :] += tmp[sy, :, :] * weight
        out16[y, :, :] *= 1.0 / total
    # Go's ftou rounds to uint16, then NRGBA stores the high byte.
    quantized = np.clip(np.floor(out16*65535.0 + 0.5), 0, 65535).astype(np.uint32)
    return (quantized >> 8).astype(np.uint8)


def _fit(ref, cand):
    h, w = ref.shape[:2]
    ch, cw = cand.shape[:2]
    if cw != w:
        nh = max(1, _round(ch * w / cw))
        cand = _catmull_rom_resize(cand, w, nh)
        ch = nh
    height = max(h, ch)
    r = np.full((height, w, 3), 255, dtype=np.uint8)
    c = np.full_like(r, 255)
    r[(height-h)//2:(height-h)//2+h] = ref
    c[(height-ch)//2:(height-ch)//2+ch] = cand
    return r, c


def _ink_box(rgb):
    y, x = np.where((1.0-rgb.astype(np.float64)/255).max(axis=2) > 0.2)
    if not len(x): return 0, 0, 0, 0
    return int(x.min()), int(y.min()), int(x.max()+1), int(y.max()+1)


def _crop_ink(rgb, gray=False):
    if gray:
        lum = (0.299*rgb[:,:,0] + 0.587*rgb[:,:,1] + 0.114*rgb[:,:,2]) / 255.0
        ink = np.repeat((1.0-lum)[:,:,None], 3, axis=2)
    else:
        ink = 1.0-rgb.astype(np.float64)/255.0
    y, x = np.where(ink.max(axis=2) > 0.2)
    if not len(x): return np.zeros((1,1,3), dtype=np.float64), (0,0)
    x0, y0 = int(x.min()), int(y.min())
    return ink[y0:int(y.max())+1, x0:int(x.max())+1], (x0,y0)


def _centroid(ink):
    mass = ink.max(axis=2)
    total = float(mass.sum())
    if total <= 0: return 0.0, 0.0
    ys, xs = np.indices(mass.shape)
    return float((ys*mass).sum()/total), float((xs*mass).sum()/total)


def _levels(canvas):
    final = min(max(8, min(1024, _round(.95*canvas))), canvas)
    if canvas < 32: return [canvas]
    levels, g = [], 32
    while g <= final:
        levels.append(g); g *= 2
    if not levels: levels = [32]
    elif levels[-1] < final: levels.append(final)
    return levels


def _trunc_div2(value):
    return value // 2 if value >= 0 else -((-value) // 2)


def _grid(ink, canvas, n):
    h,w = ink.shape[:2]
    integ = np.pad(ink, ((1,0),(1,0),(0,0))).cumsum(1).cumsum(0)
    edges = np.arange(n+1, dtype=np.int64)*canvas//n
    ox,oy=_trunc_div2(canvas-w),_trunc_div2(canvas-h)
    x0,x1=np.clip(edges[:-1]-ox,0,w),np.clip(edges[1:]-ox,0,w)
    y0,y1=np.clip(edges[:-1]-oy,0,h),np.clip(edges[1:]-oy,0,h)
    sums=(integ[y1[:,None],x1[None,:]]-integ[y0[:,None],x1[None,:]]
          -integ[y1[:,None],x0[None,:]]+integ[y0[:,None],x0[None,:]])
    area=np.diff(edges)[:,None]*np.diff(edges)[None,:]
    return sums/area[:,:,None]


def _overlap(a,b,dy,dx):
    n=a.shape[0]; y0,y1=max(0,dy),min(n,n+dy); x0,x1=max(0,dx),min(n,n+dx)
    if y1<=y0 or x1<=x0: return [0.,0.,0.]
    av=a[y0:y1,x0:x1]; bv=b[y0-dy:y1-dy,x0-dx:x1-dx]
    inter=np.minimum(av,bv).sum((0,1),dtype=np.float64)
    union=np.maximum(av,bv).sum((0,1),dtype=np.float64)
    union += a.sum((0,1),dtype=np.float64)-av.sum((0,1),dtype=np.float64)
    return [float(inter[i]/union[i]) if union[i]>1e-9 else 0. for i in range(3)]


def _better(score,dx,dy,best,bx,by):
    if score > best+1e-12: return True
    if abs(score-best)<=1e-12:
        m,bm=abs(dx)+abs(dy),abs(bx)+abs(by)
        return m<bm or (m==bm and (dy<by or (dy==by and dx<bx)))
    return False


def _align(a,b,canvas,levels,start):
    grids=[(_grid(a,canvas,n),_grid(b,canvas,n)) for n in levels]
    window=max(1,_round((min(16,max(0,_round(.1*32)))/32)*levels[0]))
    starts=[("origin",0,0)]
    if start!=(0,0): starts.append(("centroid",start[1],start[0]))
    runs=[]; best_run=None; best_score=-1.; best_x=best_y=0
    for name,sx,sy in starts:
        shift_x,shift_y=sx,sy
        for li,(ga,gb) in enumerate(grids):
            n=levels[li]; cx,cy,win=sx,sy,window
            if li:
                prev=levels[li-1]; cx,cy=_round(shift_x*n/prev),_round(shift_y*n/prev); win=min(2,n)
            level_best=-1.; lx,ly=cx,cy
            for dy in range(cy-win,cy+win+1):
                for dx in range(cx-win,cx+win+1):
                    ch=_overlap(ga,gb,dy,dx); score=sum(ch)/3
                    if _better(score,dx,dy,level_best,lx,ly): level_best,lx,ly,channels=score,dx,dy,ch
            shift_x,shift_y=lx,ly
        run={"name":name,"score":sum(channels)/3,"dx":_round(shift_x*canvas/levels[-1]),
             "dy":_round(shift_y*canvas/levels[-1]),"channels":channels}
        runs.append(run)
        if _better(run["score"],shift_x,shift_y,best_score,best_x,best_y):
            best_run,best_score,best_x,best_y=run,run["score"],shift_x,shift_y
    return best_run,runs


def _measure(ref,cand,mm):
    rb,cb=_ink_box(ref),_ink_box(cand)
    rw,rh=rb[2]-rb[0],rb[3]-rb[1]; cw,ch=cb[2]-cb[0],cb[3]-cb[1]
    rh0,rw0=ref.shape[:2]; ch0,cw0=cand.shape[:2]
    shares=(rw/rw0 if rw0 else 0,rh/rh0 if rh0 else 0,cw/cw0 if cw0 else 0,ch/ch0 if ch0 else 0)
    mrw,mrh,mcw,mch=mm
    if mrw>0 and mrh>0 and mcw>0 and mch>0:
        unit="mm"; irw,irh,icw,ich=shares[0]*mrw,shares[1]*mrh,shares[2]*mcw,shares[3]*mch
        sx=icw/irw if irw>0 else 1.; sy=ich/irh if irh>0 else 1.; dev=max(abs(1-sx),abs(1-sy))
    else:
        unit="aspect"; irw,irh,icw,ich=map(float,(rw,rh,cw,ch)); sx=sy=0.
    if irw>0 and irh>0 and icw>0 and ich>0:
        ar,ac=irw/irh,icw/ich
        adev=abs(ac/ar-1) if ar>0 else 0.
    else:
        ar=ac=adev=0.
    if unit=="aspect": dev=adev
    return dict(scale_x=sx,scale_y=sy,scale_dev=dev,size_unit=unit,aspect_ref=ar,aspect_cand=ac,aspect_dev=adev,
                ink_ref_w=irw,ink_ref_h=irh,ink_cand_w=icw,ink_cand_h=ich,
                share_ref_w=shares[0],share_ref_h=shares[1],share_cand_w=shares[2],share_cand_h=shares[3])


def compare_images(reference: Image.Image, candidate: Image.Image, canvas_mm=(0,0,0,0)) -> dict:
    """canvas_mm ordering: (reference width/height, candidate width/height)."""
    ref,cand=_rgb(reference),_rgb(candidate)
    if not ref.shape[0] or not ref.shape[1] or not cand.shape[0] or not cand.shape[1]: raise ValueError("empty input image")
    size=_measure(ref,cand,canvas_mm)
    ref,cand=_fit(ref,cand)
    a,ro=_crop_ink(ref); b,co=_crop_ink(cand)
    canvas=max(24,min(512,max(a.shape[0],a.shape[1],b.shape[0],b.shape[1])))
    levels=_levels(canvas); y1,x1=_centroid(a); y2,x2=_centroid(b)
    start=(_round((y1-y2)*levels[0]/canvas),_round((x1-x2)*levels[0]/canvas))
    best,runs=_align(a,b,canvas,levels,start)
    dx,dy=best["dx"]+ro[0]-co[0],best["dy"]+ro[1]-co[1]
    ga,go=_crop_ink(ref,True); gb,gbo=_crop_ink(cand,True); grid=levels[-1]
    gray=sum(_overlap(_grid(ga,canvas,grid),_grid(gb,canvas,grid),
        _round((dy-(go[1]-gbo[1]))*grid/canvas),_round((dx-(go[0]-gbo[0]))*grid/canvas)))/3
    return dict(similarity=best["score"],channels=best["channels"],gray=gray,dx=dx,dy=dy,
                runs=runs,**{k:size[k] for k in ("scale_x","scale_y","scale_dev","size_unit","aspect_ref","aspect_cand",
                "ink_ref_w","ink_ref_h","ink_cand_w","ink_cand_h")}, **{k:size[k] for k in ("aspect_dev","share_ref_w","share_ref_h","share_cand_w","share_cand_h")},canvas=canvas,grid=grid)


def _channel_ink(rgb,detail):
    if detail=="gray": return 255-(.299*rgb[:,:,0]+.587*rgb[:,:,1]+.114*rgb[:,:,2]).astype(np.uint8)
    idx={"r":0,"g":1,"b":2}
    if detail in idx: return 255-rgb[:,:,idx[detail]].astype(np.int16)
    if detail in ("rg","rb","gb"): return np.maximum.reduce([255-rgb[:,:,idx[c]].astype(np.int16) for c in detail])
    return (255-rgb.astype(np.int16)).max(axis=2)


def _overlay_single(ref,cand,dx,dy,detail):
    h,w=ref.shape[:2]; yy,xx=np.indices((h,w)); cx,cy=xx-dx,yy-dy
    valid=(cx>=0)&(cx<cand.shape[1])&(cy>=0)&(cy<cand.shape[0])
    sampled=np.full_like(ref,255); sampled[valid]=cand[cy[valid],cx[valid]]
    ri,ci=_channel_ink(ref,detail),_channel_ink(sampled,detail); out=np.zeros_like(ref)
    shared=(ri>51)&(ci>51); ro=(ri>51)&~shared; co=(ci>51)&~shared
    out[shared]=255; out[ro,0]=255; out[ro,2]=255; out[co,1]=255
    return out


def make_overlay(ref: Image.Image, cand: Image.Image, dx: int, dy: int, detail: str) -> Image.Image:
    r,c=_fit(_rgb(ref),_rgb(cand))
    if detail=="all":
        ps=[_overlay_single(r,c,dx,dy,d) for d in ("gray","r","g","b")]; h,w=ps[0].shape[:2]
        out=np.zeros((2*h,2*w,3),dtype=np.uint8)
        for i,p in enumerate(ps): out[(i//2)*h:(i//2+1)*h,(i%2)*w:(i%2+1)*w]=p
        return Image.fromarray(out,"RGB")
    if detail not in ("gray","r","g","b","rg","rb","gb"): raise ValueError("invalid detail")
    return Image.fromarray(_overlay_single(r,c,dx,dy,detail),"RGB")


def _crop_percent(img,left,top,right,bottom):
    h,w=img.shape[:2]; x0,y0=_round(left*w/100),_round(top*h/100); x1,y1=_round(right*w/100),_round(bottom*h/100)
    x0,y0=max(0,min(w-1,x0)),max(0,min(h-1,y0)); x1,y1=max(x0+1,min(w,x1)),max(y0+1,min(h,y1))
    return img[y0:y1,x0:x1]


def worst_region(ref: Image.Image,cand: Image.Image) -> tuple[float,float,float,float]:
    r,c=_fit(_rgb(ref),_rgb(cand)); worst=math.inf; region=(0.,0.,100.,100.)
    for row in range(2):
        for col in range(2):
            left,top=col*50.,row*50.
            rr=Image.fromarray(_crop_percent(r,left,top,left+50,top+50),"RGB")
            cc=Image.fromarray(_crop_percent(c,left,top,left+50,top+50),"RGB")
            score=compare_images(rr,cc)["similarity"]
            if score<worst: worst,region=score,(left,top,left+50,top+50)
    return region
