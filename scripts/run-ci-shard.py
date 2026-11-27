"""Run a package shard, expanding pkg@index/total against the built test inventory."""

import json
import re
import subprocess
import sys
from time import monotonic


def run_slice(pkg, index, total, flags):
    cmd = ["go", "test", *flags]
    listing = subprocess.check_output([*cmd, "-list", ".", pkg], text=True)
    lines = listing.splitlines()
    if not lines or lines[-1].split()[:2] != ["ok", pkg]:
        raise ValueError(f"incomplete test inventory for {pkg}: {listing}")
    names = []
    for name in lines[:-1]:
        if not re.fullmatch(r"(?:Test|Example|Benchmark|Fuzz)\w*", name):
            raise ValueError(f"unexpected test inventory entry: {name!r}")
        if not name.startswith("Benchmark"):  # go test runs benchmarks only with -bench.
            names.append(name)
    if not 0 <= index < total <= len(names) or len(set(names)) != len(names):
        raise ValueError(f"invalid split {index}/{total} of {len(names)} tests in {pkg}")
    selected = sorted(names)[index::total]
    pattern = "^(" + "|".join(selected) + ")$"
    print(f"{pkg} slice {index}/{total}: {len(selected)} of {len(names)} tests", flush=True)
    start = monotonic()
    result = subprocess.run([*cmd, "-json", "-run", pattern, pkg], stdout=subprocess.PIPE, text=True)
    if result.returncode:
        sys.stderr.write(result.stdout)
        result.check_returncode()
    started = set()
    for line in result.stdout.splitlines():
        event = json.loads(line)
        if event.get("Action") == "run" and "/" not in event["Test"]:
            started.add(event["Test"])
    if started != set(selected):
        raise ValueError(f"{pkg} execution differs from its inventory: {started ^ set(selected)}")
    print(f"ok\t{pkg}@{index}/{total}\t{len(started)} tests\t{monotonic() - start:.3f}s")


def main(args):
    separator = args.index("--")
    flags, assignments = args[:separator], args[separator + 1:]
    if not assignments:
        raise ValueError("empty package shard")
    packages = [item for item in assignments if "@" not in item]
    if packages:
        subprocess.run(["go", "test", *flags, *packages], check=True)
    for item in assignments:
        if "@" in item:
            pkg, part = item.split("@")
            index, total = map(int, part.split("/"))
            run_slice(pkg, index, total, flags)


if __name__ == "__main__":
    main(sys.argv[1:])
