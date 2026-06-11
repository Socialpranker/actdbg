#!/usr/bin/env python3
"""Generate README assets for actdbg: assets/demo.gif + assets/social-preview.png.

Pure Pillow, frame-by-frame. ASCII-only text (system DejaVu fonts carry no emoji
glyphs); checkmarks / crosses / arrows are drawn with line segments instead.

Usage:
    python3 tools/gen_demo.py [outdir]          # default outdir: assets/
"""
import os
import sys

from PIL import Image, ImageDraw, ImageFont

# ---------------------------------------------------------------- palette ----
BG     = "#0b0d10"
PANEL  = "#14181d"
TEXT   = "#e6e9ec"
GRAY   = "#8a949e"
RED    = "#f7775a"
GREEN  = "#7df0a8"
AMBER  = "#f5d76e"
BLUE   = "#5ab0f7"
DOTS   = ("#ff5f57", "#febc2e", "#28c840")

BORDER   = "#232a33"
CHIP_BG  = "#1d242c"
AMBER_BG = "#2a2310"
BLUE_BG  = "#13222e"
GREEN_BG = "#15291c"
ED_BG    = "#181e25"

FONT_DIR = "/usr/share/fonts/truetype/dejavu/"


def F(name, size):
    return ImageFont.truetype(FONT_DIR + name, size)


MONO    = F("DejaVuSansMono.ttf", 15)
MONO_B  = F("DejaVuSansMono-Bold.ttf", 15)
MONO_SM = F("DejaVuSansMono.ttf", 13)
SANS    = F("DejaVuSans.ttf", 15)
BIG     = F("DejaVuSans-Bold.ttf", 27)
BIG_SUB = F("DejaVuSans.ttf", 16)

W, H = 760, 520
X0, Y0 = 36, 74          # content origin inside the terminal window
LH = 24                  # default line height
GLYPH_W = 24             # width of the drawn-glyph cell


# ------------------------------------------------------------ drawn glyphs ---
def draw_check(d, x, cy, color=GREEN, s=1.0):
    d.line([(x, cy + 1 * s), (x + 4 * s, cy + 5 * s)], fill=color, width=3)
    d.line([(x + 4 * s, cy + 5 * s), (x + 11 * s, cy - 5 * s)], fill=color, width=3)


def draw_cross(d, x, cy, color=RED, s=1.0):
    d.line([(x + 1 * s, cy - 5 * s), (x + 10 * s, cy + 5 * s)], fill=color, width=3)
    d.line([(x + 10 * s, cy - 5 * s), (x + 1 * s, cy + 5 * s)], fill=color, width=3)


def draw_arrow(d, x, cy, color=BLUE):  # "running" wedge
    d.line([(x + 2, cy - 5), (x + 9, cy)], fill=color, width=3)
    d.line([(x + 9, cy), (x + 2, cy + 5)], fill=color, width=3)


# ---------------------------------------------------------- window chrome ----
def window(d, chip):
    d.rounded_rectangle([10, 14, W - 10, H - 14], radius=14, fill=PANEL,
                        outline=BORDER, width=1)
    for i, c in enumerate(DOTS):
        cx = 36 + i * 22
        d.ellipse([cx - 6, 32, cx + 6, 44], fill=c)
    title = "actdbg - debug GitHub Actions locally"
    tw = d.textlength(title, MONO_SM)
    d.text(((W - tw) / 2, 31), title, font=MONO_SM, fill=GRAY)
    ctw = d.textlength(chip, MONO_SM)
    cx1 = W - 26
    cx0 = cx1 - ctw - 18
    d.rounded_rectangle([cx0, 27, cx1, 49], radius=11, fill=CHIP_BG,
                        outline=BORDER, width=1)
    d.text((cx0 + 9, 31), chip, font=MONO_SM, fill=BLUE)
    d.line([11, 58, W - 11, 58], fill=BORDER)


# -------------------------------------------------------------- renderer -----
def render(lines, chip, cursor=False):
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)
    window(d, chip)
    y = Y0
    last_end = None
    for ln in lines:
        t = ln["t"]
        if t == "gap":
            y += ln.get("h", 10)
        elif t == "seg":
            cy = y + LH // 2
            x = X0
            g = ln.get("glyph")
            if g:
                {"check": draw_check, "cross": draw_cross,
                 "arrow": draw_arrow}[g](d, x, cy)
                x += GLYPH_W
            for txt, col in ln["segs"]:
                d.text((x, y + 3), txt, font=MONO, fill=col)
                x += d.textlength(txt, MONO)
            last_end = (x, y)
            y += LH
        elif t == "plaque":
            txt = ln["text"]
            col = ln.get("color", AMBER)
            bgc = ln.get("bg", AMBER_BG)
            tw = d.textlength(txt, MONO)
            y += 4
            d.rounded_rectangle([X0, y, X0 + tw + 22, y + 28], radius=8,
                                fill=bgc, outline=col, width=1)
            d.text((X0 + 11, y + 5), txt, font=MONO, fill=col)
            y += 36
        elif t == "pill":
            txt = ln["text"]
            tw = d.textlength(txt, MONO_B)
            pw = tw + 34
            x0p = (W - pw) // 2 if ln.get("center") else X0 + ln.get("indent", 0)
            y += 6
            d.rounded_rectangle([x0p, y, x0p + pw, y + 32], radius=16,
                                fill=BLUE_BG, outline=BLUE, width=2)
            d.text((x0p + 17, y + 7), txt, font=MONO_B, fill=BLUE)
            y += 44
        elif t == "editor":
            rows = ln["rows"]
            eh = len(rows) * 21 + 14
            d.rounded_rectangle([X0, y + 2, W - 60, y + 2 + eh], radius=8,
                                fill=ED_BG, outline=BORDER, width=1)
            yy = y + 10
            for txt, col, hl in rows:
                if hl:
                    d.rectangle([X0 + 2, yy - 1, W - 62, yy + 19], fill=GREEN_BG)
                d.text((X0 + 14, yy + 1), txt, font=MONO, fill=col)
                yy += 21
            y += eh + 10
        elif t == "big":
            d.text((X0, y), ln["main"], font=BIG, fill=GREEN)
            w1 = d.textlength(ln["main"], BIG)
            d.text((X0 + w1 + 14, y + 11), ln["sub"], font=BIG_SUB, fill=GRAY)
            y += 44
            d.text((X0, y), ln["note"], font=SANS, fill=GRAY)
            y += 26
    if cursor and last_end:
        cx, cyl = last_end
        d.rectangle([cx + 2, cyl + 3, cx + 11, cyl + 20], fill=TEXT)
    return img


# ------------------------------------------------------------- timeline ------
frames, durs = [], []


def emit(lines, chip, cursor=False, ms=160):
    frames.append(render(list(lines), chip, cursor))
    durs.append(ms)


def type_line(lines, chip, prefix_segs, text, color=TEXT, ms=115, step=1):
    idxs = list(range(step, len(text) + 1, step))
    if not idxs or idxs[-1] != len(text):
        idxs.append(len(text))
    for i in idxs:
        cur = lines + [{"t": "seg", "segs": prefix_segs + [(text[:i], color)]}]
        emit(cur, chip, cursor=True, ms=ms)
    lines.append({"t": "seg", "segs": prefix_segs + [(text, color)]})


def blink(lines, chip, n=2, ms=300):
    for k in range(n * 2):
        emit(lines, chip, cursor=(k % 2 == 1), ms=ms)


def build_scenes():
    marks = []

    # ---- scene 1: run -------------------------------------------------------
    chip = "1/3  run"
    L = []
    emit(L + [{"t": "seg", "segs": [("$ ", GRAY)]}], chip, cursor=True, ms=350)
    type_line(L, chip, [("$ ", GRAY)], "actdbg run", ms=115)
    blink(L, chip, n=1, ms=170)
    chk = [("[test] ", GRAY), ("Checkout", TEXT)]
    L.append({"t": "seg", "glyph": "arrow", "segs": chk})
    emit(L, chip, ms=400)
    L[-1] = {"t": "seg", "glyph": "check", "segs": chk}
    emit(L, chip, ms=330)
    L.append({"t": "seg", "glyph": "arrow", "segs": [("[test] ", GRAY), ("Build", TEXT)]})
    emit(L, chip, ms=500)
    L[-1] = {"t": "seg", "glyph": "cross", "segs": [("[test] ", GRAY), ("Build", RED)]}
    emit(L, chip, ms=400)
    L.append({"t": "seg", "segs": [("   | error: pnpm: command not found", GRAY)]})
    emit(L, chip, ms=550)
    L.append({"t": "gap", "h": 6})
    L.append({"t": "plaque",
              "text": 'STOPPED: step 2/5 "Build" failed - container kept alive'})
    emit(L, chip, ms=700)
    L.append({"t": "seg", "segs": [
        ("   time-travel ready: ", GRAY), ("back N", BLUE), ("  -  ", GRAY),
        ("rerun --from 2", BLUE), ("  -  ", GRAY), ("diff", BLUE)]})
    emit(L, chip, ms=1600)
    marks.append(len(frames) - 1)

    # ---- scene 2: shell ------------------------------------------------------
    chip = "2/3  shell"
    L = []
    type_line(L, chip, [("$ ", GRAY)], "actdbg shell", ms=110)
    L.append({"t": "gap", "h": 4})
    L.append({"t": "plaque", "color": BLUE, "bg": BLUE_BG,
              "text": 'shell at failed step 2/5 "Build" - 14 env vars reconstructed'})
    emit(L, chip, ms=750)
    prompt = [("root@act-ci-test", GREEN), (":", GRAY), ("/repo", BLUE), ("# ", TEXT)]
    L.append({"t": "seg", "segs": prompt})
    emit(L, chip, cursor=True, ms=420)
    L.pop()
    type_line(L, chip, prompt, "which pnpm", ms=110)
    L.append({"t": "seg", "segs": [("not found", GRAY)]})
    emit(L, chip, ms=600)
    L.append({"t": "gap", "h": 8})
    L.append({"t": "pill", "center": True,
              "text": "your shell, inside CI, at the failure"})
    emit(L, chip, ms=850)
    L.append({"t": "seg", "segs": prompt})
    blink(L, chip, n=2, ms=320)
    emit(L, chip, cursor=True, ms=700)
    marks.append(len(frames) - 1)

    # ---- scene 3: fix & rerun ------------------------------------------------
    chip = "3/3  fix & rerun"
    L = []
    type_line(L, chip, [("$ ", GRAY)], "vim .github/workflows/ci.yml", ms=95, step=2)
    L.append({"t": "editor", "rows": [
        ("  steps:",                              GRAY,  False),
        ("    - uses: actions/checkout@v4",       GRAY,  False),
        ("+   - run: npm i -g pnpm",              GREEN, True),
        ("    - run: pnpm install && pnpm build", GRAY,  False),
    ]})
    emit(L, chip, ms=1000)
    type_line(L, chip, [("$ ", GRAY)], "actdbg rerun --from 2", ms=95, step=2)
    L.append({"t": "seg", "segs": [("<< restored state after step 1", BLUE)]})
    emit(L, chip, ms=550)
    L.append({"t": "seg", "glyph": "check", "segs": [("step 2 done", TEXT)]})
    emit(L, chip, ms=480)
    L.append({"t": "seg", "glyph": "check", "segs": [("step 3 done", TEXT)]})
    emit(L, chip, ms=480)
    L.append({"t": "gap", "h": 10})
    L.append({"t": "big", "main": "green in 14s", "sub": "(full rerun: 6m)",
              "note": 'no pushes. no "fix ci" commits.'})
    emit(L, chip, ms=2600)
    marks.append(len(frames) - 1)
    return marks


def save_gif(path):
    # shared palette built from one rich contact sheet (scene finals)
    sheet = Image.new("RGB", (W, H * 3), BG)
    rich = [f for f in (frames[i] for i in SCENE_ENDS)]
    for i, f in enumerate(rich):
        sheet.paste(f, (0, i * H))
    base = sheet.quantize(colors=128, method=Image.Quantize.MEDIANCUT,
                          dither=Image.Dither.NONE)
    pal = [f.quantize(palette=base, dither=Image.Dither.NONE) for f in frames]
    pal[0].save(path, save_all=True, append_images=pal[1:], duration=durs,
                loop=0, optimize=True)


# --------------------------------------------------------- social preview ----
def social(path):
    WP, HP = 1280, 640
    img = Image.new("RGB", (WP, HP), BG)
    d = ImageDraw.Draw(img)
    title_f = F("DejaVuSans-Bold.ttf", 120)
    sub_f   = F("DejaVuSans.ttf", 30)
    m17     = F("DejaVuSansMono.ttf", 17)
    m14     = F("DejaVuSansMono.ttf", 14)
    m23     = F("DejaVuSansMono.ttf", 23)

    # left: wordmark + subtitle
    x, y = 72, 168
    d.text((x, y), "act", font=title_f, fill=TEXT)
    d.text((x + d.textlength("act", title_f), y), "dbg", font=title_f, fill=RED)
    d.text((x + 6, y + 158), "a debugger for GitHub Actions,", font=sub_f, fill=GRAY)
    d.text((x + 6, y + 200), "locally", font=sub_f, fill=TEXT)

    # right: simplified terminal (scene 1)
    tx0, ty0, tx1, ty1 = 640, 96, 1216, 536
    d.rounded_rectangle([tx0, ty0, tx1, ty1], radius=16, fill=PANEL,
                        outline=BORDER, width=1)
    for i, c in enumerate(DOTS):
        cx = tx0 + 28 + i * 24
        d.ellipse([cx - 7, ty0 + 21, cx + 7, ty0 + 35], fill=c)
    tt = "actdbg run"
    d.text(((tx0 + tx1 - d.textlength(tt, m14)) / 2, ty0 + 21), tt,
           font=m14, fill=GRAY)
    d.line([tx0 + 1, ty0 + 48, tx1 - 1, ty0 + 48], fill=BORDER)

    lx, ly, lh = tx0 + 28, ty0 + 68, 29
    d.text((lx, ly), "$ ", font=m17, fill=GRAY)
    d.text((lx + d.textlength("$ ", m17), ly), "actdbg run", font=m17, fill=TEXT)
    ly += lh
    cy = ly + 10
    draw_check(d, lx, cy, s=1.15)
    d.text((lx + 26, ly), "[test] ", font=m17, fill=GRAY)
    d.text((lx + 26 + d.textlength("[test] ", m17), ly), "Checkout",
           font=m17, fill=TEXT)
    ly += lh
    cy = ly + 10
    draw_cross(d, lx, cy, s=1.15)
    d.text((lx + 26, ly), "[test] ", font=m17, fill=GRAY)
    d.text((lx + 26 + d.textlength("[test] ", m17), ly), "Build",
           font=m17, fill=RED)
    ly += lh
    d.text((lx, ly), "  | error: pnpm: command not found", font=m17, fill=GRAY)
    ly += lh + 12
    st = 'STOPPED: step 2/5 "Build" failed - container kept alive'
    stw = d.textlength(st, m14)
    d.rounded_rectangle([lx, ly, lx + stw + 22, ly + 30], radius=8,
                        fill=AMBER_BG, outline=AMBER, width=1)
    d.text((lx + 11, ly + 6), st, font=m14, fill=AMBER)
    ly += 30 + 16
    d.text((lx, ly), "time-travel: ", font=m17, fill=GRAY)
    xx = lx + d.textlength("time-travel: ", m17)
    for t, c in (("back N", BLUE), ("  -  ", GRAY), ("rerun --from 2", BLUE),
                 ("  -  ", GRAY), ("diff", BLUE)):
        d.text((xx, ly), t, font=m17, fill=c)
        xx += d.textlength(t, m17)
    ly += lh + 14
    d.text((lx, ly), "$ actdbg shell", font=m17, fill=TEXT)
    cw = d.textlength("$ actdbg shell", m17)
    d.rectangle([lx + cw + 4, ly + 1, lx + cw + 15, ly + 21], fill=TEXT)

    # bottom: pipeline strip
    parts = [("run", TEXT), ("  ->  ", GRAY), ("stop at failure", AMBER),
             ("  ->  ", GRAY), ("shell", BLUE), ("  ->  ", GRAY),
             ("rerun --from N", GREEN)]
    total = sum(d.textlength(t, m23) for t, _ in parts)
    xx = (WP - total) / 2
    for t, c in parts:
        d.text((xx, 576), t, font=m23, fill=c)
        xx += d.textlength(t, m23)

    img.save(path, optimize=True)


# ------------------------------------------------------------------ main -----
if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "..", "assets")
    os.makedirs(out, exist_ok=True)
    SCENE_ENDS = build_scenes()
    gif = os.path.join(out, "demo.gif")
    png = os.path.join(out, "social-preview.png")
    save_gif(gif)
    social(png)
    total_s = sum(durs) / 1000
    print(f"frames={len(frames)} duration={total_s:.1f}s scene_ends={SCENE_ENDS}")
    for p in (gif, png):
        print(f"{p}: {os.path.getsize(p) / 1024:.0f} KB")
