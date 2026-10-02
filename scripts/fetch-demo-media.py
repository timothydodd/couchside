#!/usr/bin/env python3
"""Download free-to-show demo media for a Couchside demo server.

Everything here is either Creative Commons Attribution (the Blender open
movies: credit "Blender Foundation | www.blender.org" and keep the films' own
credits) or public domain in the US (the Internet Archive items, all marked
public domain there). Public domain status is US-only; restorations and new
scores can carry their own copyright, so the items below are plain prints.

    python fetch-demo-media.py D:\\couchside-demo            # everything
    python fetch-demo-media.py /mnt/media/demo --only movies tv
    python fetch-demo-media.py /mnt/media/demo --list         # sizes, no download

Lays files out the way Couchside (and Plex) expect:

    <root>/Movies/Big Buck Bunny (2008)/Big Buck Bunny (2008).mp4
    <root>/TV/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies - S01E01 - The Clampetts Strike Oil.mp4
    <root>/Commercials/Cheerios V-8 (1960).mp4

Add Movies and TV as libraries. Commercials is filler for virtual channels,
not a library. Re-running skips finished files and resumes partial ones.
Python 3.9+, standard library only.
"""

import argparse
import json
import os
import re
import shutil
import sys
import time
import urllib.error
import urllib.request
import zipfile

UA = "Mozilla/5.0 (couchside demo fetcher; +https://couchside.app)"
NLUUG = "https://ftp.nluug.nl/pub/graphics/blender/demo/movies"  # official Blender mirror

# --- what to fetch --------------------------------------------------------------
# Each movie: (folder/file name, source). A source is ("url", url) for a direct
# file, ("zip", url) for a mirror file that's zipped, or ("ia", item, file) for
# an Internet Archive file.

MOVIES = [
    ("Big Buck Bunny (2008)", ("zip", f"{NLUUG}/BBB/bbb_sunflower_1080p_30fps_normal.mp4.zip")),
    ("Sintel (2010)", ("url", f"{NLUUG}/Sintel.2010.1080p.mkv")),  # subtitles are inside the mkv
    ("Tears of Steel (2012)", ("zip", f"{NLUUG}/ToS/tears_of_steel_1080p.mov.zip")),
    ("Night of the Living Dead (1968)", ("ia", "Night.Of.The.Living.Dead_1080p", "NightOfTheLivingDead_1080p.mp4")),
    ("His Girl Friday (1940)", ("ia", "his_girl_friday", "his_girl_friday.mp4")),
    ("The General (1926)", ("ia", "The_General_Buster_Keaton", "The_General.mp4")),
    # Low quality on purpose: shows the SD badge and the Optimize/transcode paths.
    ("Nosferatu (1922)", ("ia", "Nosferatu_most_complete_version_93_mins.", "Nosferatu_1922_Symphony_of_Horror_512kb.mp4")),
]

# Sidecar subtitles: (movie folder, language, url). Saved as "<Movie>.<lang>.srt".
SUBTITLES = [
    ("Tears of Steel (2012)", lang, f"{NLUUG}/ToS/subtitles/TOS-{src}.srt")
    for lang, src in [("en", "en"), ("es", "es"), ("de", "de"), ("fr", "fr-orig"), ("it", "it"), ("nl", "nl"), ("ja", "JP"), ("ru", "ru")]
]

# TV: the public-domain Beverly Hillbillies episodes (season 1; the copyrights
# weren't renewed). Episodes are found by their sNNeNN file names.
TV = [
    ("The Beverly Hillbillies (1962)", "The Beverly Hillbillies", "bevhill-s01e01-36"),
]

# Filler for virtual channels: public-domain 1950s/60s TV commercials from the
# Prelinger Archives.
COMMERCIALS = [
    ("Cheerios V-8 (1960)", "Cheerios1960"),
    ("A Wonderful New World of Fords (1960)", "Wonderfu1960"),
    ("1955 Chevrolet Screen Ads (1955)", "1955Chev1955"),
    ("Classic Television Commercials Part 1 (1948)", "ClassicT1948"),
    ("Classic Television Commercials Part 2 (1948)", "ClassicT1948_2"),
    ("Classic Television Commercials Part 3 (1948)", "ClassicT1948_3"),
    ("Television Commercials 1950s-1960s (1960)", "Televisi1960"),
]

# --- helpers --------------------------------------------------------------------


def request(url, **headers):
    return urllib.request.Request(url, headers={"User-Agent": UA, **headers})


def ia_files(item):
    """The Internet Archive's file list for an item."""
    with urllib.request.urlopen(request(f"https://archive.org/metadata/{item}"), timeout=60) as r:
        return json.load(r).get("files", [])


def ia_url(item, name):
    return f"https://archive.org/download/{item}/{urllib.request.quote(name)}"


def best_mp4(files):
    """The largest h.264 mp4 an item has (its best copy that browsers play)."""
    mp4 = [f for f in files if f["name"].lower().endswith(".mp4")]
    return max(mp4, key=lambda f: int(f.get("size", 0)), default=None)


def remote_size(url):
    try:
        with urllib.request.urlopen(request(url, Range="bytes=0-0"), timeout=60) as r:
            rng = r.headers.get("Content-Range", "")
            return int(rng.rsplit("/", 1)[1]) if "/" in rng else int(r.headers.get("Content-Length", 0))
    except (urllib.error.URLError, ValueError, OSError):
        return 0


def download(url, dest, size_hint=0, tries=5):
    """Download url to dest, resuming a .part file. Skips a finished dest."""
    if os.path.exists(dest) and os.path.getsize(dest) > 0:
        print(f"  have  {os.path.basename(dest)}")
        return True
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    part = dest + ".part"
    for attempt in range(1, tries + 1):
        have = os.path.getsize(part) if os.path.exists(part) else 0
        try:
            req = request(url, **({"Range": f"bytes={have}-"} if have else {}))
            with urllib.request.urlopen(req, timeout=120) as r:
                if have and r.status != 206:  # server ignored the range: start over
                    have = 0
                total = have + int(r.headers.get("Content-Length", 0)) or size_hint
                with open(part, "ab" if have else "wb") as out:
                    done, last = have, 0.0
                    while chunk := r.read(1 << 20):
                        out.write(chunk)
                        done += len(chunk)
                        if time.time() - last > 1:
                            last = time.time()
                            pct = f"{done * 100 // total:3d}%" if total else ""
                            print(f"\r  get   {os.path.basename(dest)}  {done / 1e6:,.0f} MB {pct}   ", end="", flush=True)
            os.replace(part, dest)
            print(f"\r  done  {os.path.basename(dest)}  {os.path.getsize(dest) / 1e6:,.0f} MB        ")
            return True
        except urllib.error.HTTPError as e:
            if e.code == 416:  # range past the end: the .part is complete
                os.replace(part, dest)
                return True
            print(f"\n  HTTP {e.code} for {url} (try {attempt}/{tries})")
            if e.code in (403, 404):
                return False
        except (urllib.error.URLError, OSError, TimeoutError) as e:
            print(f"\n  {e} (try {attempt}/{tries})")
        time.sleep(min(60, 5 * attempt))
    return False


def unzip_one(zpath, dest):
    """Extract the single video in a zip to dest, then delete the zip."""
    with zipfile.ZipFile(zpath) as z:
        member = max(z.infolist(), key=lambda i: i.file_size)
        with z.open(member) as src, open(dest + ".part", "wb") as out:
            shutil.copyfileobj(src, out, 1 << 20)
    os.replace(dest + ".part", dest)
    os.remove(zpath)


def ext_of(name):
    return os.path.splitext(name.removesuffix(".zip"))[1].lower()


def safe(name):
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()


# --- sections -------------------------------------------------------------------


def plan_movies(root):
    for title, src in MOVIES:
        folder = os.path.join(root, "Movies", title)
        if src[0] == "ia":
            url = ia_url(src[1], src[2])
            yield url, os.path.join(folder, title + ext_of(src[2])), None
        else:
            url = src[1]
            dest = os.path.join(folder, title + ext_of(url))
            yield url, dest, (dest + ".zip" if src[0] == "zip" else None)
    for title, lang, url in SUBTITLES:
        yield url, os.path.join(root, "Movies", title, f"{title}.{lang}.srt"), None


RE_EP = re.compile(r"s(\d+)e(\d+)[-_ ]*(.*)\.mp4$", re.I)


def plan_tv(root):
    for folder, show, item in TV:
        for f in ia_files(item):
            m = RE_EP.search(f["name"])
            if not m or f["name"].lower().endswith("_512kb.mp4"):
                continue
            season, ep, rest = int(m[1]), int(m[2]), m[3].replace("_", " ").replace("-", " ").strip()
            name = f"{show} - S{season:02d}E{ep:02d}" + (f" - {safe(rest)}" if rest else "") + ".mp4"
            yield ia_url(item, f["name"]), os.path.join(root, "TV", folder, f"Season {season:02d}", name), None


def plan_commercials(root):
    for title, item in COMMERCIALS:
        f = best_mp4(ia_files(item))
        if f:
            yield ia_url(item, f["name"]), os.path.join(root, "Commercials", title + ".mp4"), None
        else:
            print(f"  skip  {title}: no mp4 in {item}")


SECTIONS = {"movies": plan_movies, "tv": plan_tv, "commercials": plan_commercials}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("root", help="folder to fill (created if missing)")
    ap.add_argument("--only", nargs="+", choices=SECTIONS, default=list(SECTIONS), help="sections to fetch")
    ap.add_argument("--list", action="store_true", help="show what would be fetched, with sizes")
    args = ap.parse_args()

    root = os.path.abspath(args.root)
    failed, total = [], 0
    for section in args.only:
        print(f"\n== {section}")
        for url, dest, zpath in SECTIONS[section](root):
            if args.list:
                size = remote_size(url)
                total += size
                print(f"  {size / 1e6:8,.0f} MB  {os.path.relpath(dest, root)}")
                continue
            if os.path.exists(dest):
                print(f"  have  {os.path.basename(dest)}")
                continue
            if zpath:
                if download(url, zpath):
                    print(f"  unzip {os.path.basename(zpath)}")
                    unzip_one(zpath, dest)
                else:
                    failed.append(url)
            elif not download(url, dest):
                failed.append(url)
    if args.list:
        print(f"\nTotal about {total / 1e9:,.1f} GB")
    if failed:
        print("\nFailed (re-run to retry):\n  " + "\n  ".join(failed))
        sys.exit(1)
    if not args.list:
        print(f"\nDone. Add {os.path.join(root, 'Movies')} and {os.path.join(root, 'TV')} as libraries in Couchside.")


if __name__ == "__main__":
    main()
