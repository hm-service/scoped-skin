#!/usr/bin/env python3
"""把官方的默认皮肤抓进仓库的 `builtin/`（Go 版把它嵌进二进制）。

数据源是 misode/mcmeta 通过 jsDelivr 镜像的客户端资源：
    assets/minecraft/textures/entity/player/{wide,slim}/<name>.png
``wide`` 就是经典（steve）模型，``slim`` 是纤细（alex）模型。

每个默认角色只取它的**官方默认模型**（见 minecraft.wiki/w/Skin#Default_skins）：
Ari / Kai / Steve / Sunny / Zuri 默认 wide，Alex / Efe / Makena / Noor 默认 slim。
"""

from __future__ import annotations

import json
import struct
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TARGET = ROOT / "builtin"
BASE = (
    "https://cdn.jsdelivr.net/gh/misode/mcmeta@assets"
    "/assets/minecraft/textures/entity/player"
)

# 角色 -> 官方默认模型（wide=steve / slim=alex）
CHARACTERS: list[tuple[str, str]] = [
    ("Steve", "wide"),
    ("Alex", "slim"),
    ("Ari", "wide"),
    ("Efe", "slim"),
    ("Kai", "wide"),
    ("Makena", "slim"),
    ("Noor", "slim"),
    ("Sunny", "wide"),
    ("Zuri", "wide"),
]

MODEL_OF = {"wide": "steve", "slim": "alex"}


def fetch(url: str) -> bytes:
    last: Exception | None = None
    for attempt in range(4):
        try:
            done = subprocess.run(  # noqa: S603 - fixed argv, trusted URL
                ["curl", "-sSL", "--max-time", "25", url], capture_output=True, check=True
            )
            if done.stdout:
                return done.stdout
            raise RuntimeError("empty response")
        except Exception as error:  # noqa: BLE001 - retry any transport hiccup
            last = error
            print(f"  retry {attempt + 1} {url}: {error}")
    raise RuntimeError(f"failed to fetch {url}") from last


def png_size(data: bytes) -> tuple[int, int]:
    if len(data) < 24 or data[12:16] != b"IHDR":
        raise ValueError("not a png")
    return struct.unpack(">II", data[16:24])


def main() -> None:
    skins = TARGET / "skins"
    if skins.exists():
        for stale in skins.glob("*.png"):
            stale.unlink()  # 目录是生成物，先清干净，免得留下上一版的多余变体
    skins.mkdir(parents=True, exist_ok=True)

    manifest: list[dict] = []
    for character, folder in CHARACTERS:
        data = fetch(f"{BASE}/{folder}/{character.lower()}.png")
        width, height = png_size(data)
        if (width, height) != (64, 64):
            raise ValueError(f"{character}/{folder}: unexpected size {width}x{height}")
        name = f"{character.lower()}-{folder}.png"
        (skins / name).write_bytes(data)
        manifest.append(
            {
                "file": f"skins/{name}",
                "model": MODEL_OF[folder],
                "tags": ["官方", character],
            }
        )
        print(f"{name:22} {width}x{height} -> {MODEL_OF[folder]}")

    (TARGET / "manifest.json").write_text(
        json.dumps({"builtin": manifest}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {TARGET / 'manifest.json'} ({len(manifest)} entries)")


if __name__ == "__main__":
    main()
