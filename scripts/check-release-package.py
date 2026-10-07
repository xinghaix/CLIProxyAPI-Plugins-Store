#!/usr/bin/env python3
"""Check a real Linux release ZIP against its library and optional legal files.

Usage: python3 scripts/check-release-package.py PLUGIN_DIR LIBRARY ZIP
"""

import hashlib
import json
from pathlib import Path
import sys
import zipfile


def main():
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    plugin, library, archive = map(Path, sys.argv[1:])
    with library.open("rb") as source:
        header = source.read(20)
    assert header[:4] == b"\x7fELF", "expected a real Linux ELF library"
    assert header[5] in (1, 2), "invalid ELF byte order"
    byteorder = "little" if header[5] == 1 else "big"
    assert int.from_bytes(header[16:18], byteorder) == 3, "expected an ELF shared object"
    expected = {library.name: library}
    for name in ("NOTICE", "LICENSE"):
        document = plugin / name
        if document.is_file():
            expected[name] = document
    with zipfile.ZipFile(archive) as package:
        assert sorted(package.namelist()) == sorted(expected), "wrong ZIP members"
        for name, source in expected.items():
            assert package.read(name) == source.read_bytes(), f"ZIP changed {name}"
    print(json.dumps({"zip": str(archive), "members": sorted(expected),
                      "sha256": hashlib.sha256(archive.read_bytes()).hexdigest()}))


if __name__ == "__main__":
    main()
