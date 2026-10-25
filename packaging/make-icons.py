#!/usr/bin/env python3
"""由 packaging/icon.svg 生成飞牛应用所需的各尺寸 PNG 图标。

用法: python3 packaging/make-icons.py
依赖: playwright（仅用于把 SVG 精确栅格化）
"""
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SVG = ROOT / "packaging" / "icon.svg"
TARGETS = [
    (ROOT / "packaging" / "ICON.PNG", 64),
    (ROOT / "packaging" / "ICON_256.PNG", 256),
    (ROOT / "packaging" / "app" / "ui" / "images" / "icon_64.png", 64),
    (ROOT / "packaging" / "app" / "ui" / "images" / "icon_256.png", 256),
]


def main() -> int:
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        print("需要 playwright：sudo pip3 install playwright && playwright install chromium", file=sys.stderr)
        return 1

    svg = SVG.read_text(encoding="utf-8").split(">", 1)[1]
    with sync_playwright() as p:
        browser = p.chromium.launch(args=["--no-sandbox"])
        for path, size in TARGETS:
            path.parent.mkdir(parents=True, exist_ok=True)
            page = browser.new_page(viewport={"width": size, "height": size}, device_scale_factor=1)
            html = (
                "<html><body style='margin:0;background:transparent'>"
                f"<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 256 256' width='{size}' height='{size}'>{svg}</svg>"
                "</body></html>"
            )
            page.set_content(html)
            page.wait_for_timeout(120)
            page.locator("svg").screenshot(path=str(path), omit_background=True)
            page.close()
            print(f"生成 {path.relative_to(ROOT)}  ({size}x{size})")
        browser.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
