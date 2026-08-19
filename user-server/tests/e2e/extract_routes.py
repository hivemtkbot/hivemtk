#!/usr/bin/env python3
"""从 gin 路由源码提取完整端点清单(METHOD + 完整路径)。

跨文件 receiver 名冲突问题通过"已知 receiver 名 -> 真实前缀"硬映射解决:
  auth / public / systemAdmin / bridgeWS -> /api
  platform                                -> /api/platform
  r / engine                              -> 顶层(空)
函数内局部 `g := base.Group("/p")` 在其后追加前缀。
"""
import re
import sys
import os

METHODS = ("GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS")
method_re = re.compile(r'([A-Za-z_]\w*)\.(%s)\("([^"]+)"' % "|".join(METHODS))
group_re = re.compile(r'([A-Za-z_]\w*)\s*:?=\s*([A-Za-z_]\w*)\.Group\("([^"]*)"\)')

# 已知 receiver -> 真实前缀(来自 router.go 顶层分组定义)
BASE_PREFIX = {
    "auth": "/api",
    "public": "/api",
    "systemAdmin": "/api",
    "bridgeWS": "/api",
    "platform": "/api/platform",
    "r": "",
    "engine": "",
}


def resolve(receiver, local_bindings):
    seen = set()
    cur = receiver
    parts = []
    while cur not in seen:
        seen.add(cur)
        if cur in local_bindings:
            base, prefix = local_bindings[cur]
            if prefix:
                parts.append(prefix)
            cur = base
        elif cur in BASE_PREFIX:
            p = BASE_PREFIX[cur]
            if p:
                parts.append(p)
            cur = None
        else:
            # 未知 receiver, 当作顶层
            cur = None
        if cur is None or cur == "":
            break
    parts.reverse()
    return "/" + "/".join(p.strip("/") for p in parts).strip("/")


def extract(text):
    # 函数内局部 Group 绑定
    local = {}
    results = []
    for line in text.splitlines():
        gm = group_re.search(line)
        if gm:
            recv, base, prefix = gm.group(1), gm.group(2), gm.group(3)
            local[recv] = (base, prefix)
            continue
        for m in method_re.finditer(line):
            recv, method, path = m.group(1), m.group(2), m.group(3)
            prefix = resolve(recv, local)
            full = (prefix.rstrip("/") + "/" + path.lstrip("/")).replace("//", "/")
            full = re.sub(r"/+", "/", full) or "/"
            results.append((method, full))
    return results


def main():
    if len(sys.argv) < 2:
        print("usage: extract_routes.py <dir-or-file> [service_label]", file=sys.stderr)
        sys.exit(1)
    target = sys.argv[1]
    label = sys.argv[2] if len(sys.argv) > 2 else target
    files = []
    if os.path.isdir(target):
        for root, _, fs in os.walk(target):
            for f in fs:
                if f.endswith(".go") and not f.endswith("_test.go"):
                    files.append(os.path.join(root, f))
    else:
        files = [target]
    out = []
    for fp in files:
        with open(fp, "r", encoding="utf-8", errors="ignore") as fh:
            try:
                txt = fh.read()
            except Exception:
                continue
            for method, path in extract(txt):
                out.append((label, method, path))
    seen = set()
    for label, method, path in out:
        key = (label, method, path)
        if key in seen:
            continue
        seen.add(key)
        print(f"{label}\t{method}\t{path}")


if __name__ == "__main__":
    main()
