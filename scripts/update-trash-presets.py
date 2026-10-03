"""Vendor the exact anime profiles and referenced formats from a reviewed Git SHA.

Usage: python scripts/update-trash-presets.py <40-character TRaSH-Guides/Guides SHA>
No credentials or live indexers are used. Review the resulting diff and run Go tests.
"""
import hashlib
import io
import json
from pathlib import Path
import re
import sys
import tarfile
import urllib.request


def main():
    if len(sys.argv) != 2 or not re.fullmatch(r"[0-9a-f]{40}", sys.argv[1]):
        raise SystemExit("Provide a full, reviewed Git commit SHA (not a moving branch).")
    revision = sys.argv[1]
    url = f"https://codeload.github.com/TRaSH-Guides/Guides/tar.gz/{revision}"
    request = urllib.request.Request(url, headers={"User-Agent": "MediaForge preset updater"})
    with urllib.request.urlopen(request, timeout=60) as response:
        archive = response.read(50 * 1024 * 1024 + 1)
    if len(archive) > 50 * 1024 * 1024:
        raise SystemExit("Upstream archive unexpectedly exceeds 50 MiB")
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as tar:
        # Read individual files without extracting archive paths to disk.
        files = {m.name.split("/", 1)[1]: tar.extractfile(m).read()
                 for m in tar if m.isfile() and "/" in m.name}
    output = Path(__file__).resolve().parents[1] / "internal/indexer/trash"
    bundle = {"repository": "https://github.com/TRaSH-Guides/Guides",
              "revision": revision, "profiles": {}}
    snapshots = {}
    for app in ("sonarr", "radarr"):
        profile_path = f"docs/json/{app}/quality-profiles/anime-remux-1080p.json"
        profile = json.loads(files[profile_path])
        formats = {json.loads(body)["trash_id"]: (path, body)
                   for path, body in files.items()
                   if path.startswith(f"docs/json/{app}/cf/") and path.endswith(".json")}
        selected = []
        snapshots[f"{app}/quality-profile.json"] = files[profile_path]
        for trash_id in profile["formatItems"].values():
            path, body = formats[trash_id]
            definition = json.loads(body)
            selected.append(definition)
            snapshots[f"{app}/{Path(path).name}"] = body
        bundle["profiles"][app + "-anime"] = {"profile": profile, "formats": selected}
    snapshots["LICENSE"] = files["LICENSE"]
    snapshots["presets.json"] = (json.dumps(bundle, indent=2, ensure_ascii=False) + "\n").encode()
    manifest = {"revision": revision, "files": {
        path: hashlib.sha256(body).hexdigest() for path, body in sorted(snapshots.items())}}
    snapshots["manifest.json"] = (json.dumps(manifest, indent=2) + "\n").encode()
    output.mkdir(parents=True, exist_ok=True)
    for path, body in snapshots.items():
        target = output / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(body)
    print(f"Pinned {len(snapshots)} files at {revision}. Run go test ./internal/indexer.")


if __name__ == "__main__":
    main()
