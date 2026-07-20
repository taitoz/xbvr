import argparse
import json
import os
import sqlite3
import ssl
import sys
import tempfile
from http.client import InvalidURL
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlparse
from urllib.request import Request, urlopen


def parse_args():
    parser = argparse.ArgumentParser(
        description="Download missing local covers for available XBVR scenes."
    )
    parser.add_argument(
        "--database",
        required=True,
        type=Path,
        help="Path to XBVR main.db",
    )
    parser.add_argument(
        "--myfiles-dir",
        required=True,
        type=Path,
        help="Path to XBVR myfiles directory",
    )
    parser.add_argument(
        "--timeout",
        default=20,
        type=float,
        help="Per-cover download timeout in seconds (default: 20)",
    )
    parser.add_argument(
        "--insecure",
        action="store_true",
        help="Allow downloads from HTTPS hosts with invalid certificates",
    )
    return parser.parse_args()


def get_first_image_url(images_json):
    try:
        images = json.loads(images_json)
    except (json.JSONDecodeError, TypeError):
        return None

    for image in images:
        if isinstance(image, dict) and image.get("url"):
            return image["url"]
    return None


def download_cover(url, destination, timeout, insecure):
    request = Request(
        quote(url, safe=":/?&=#%"),
        headers={"User-Agent": "XBVR cover repair/1.0"},
    )
    context = ssl._create_unverified_context() if insecure else None
    with urlopen(request, timeout=timeout, context=context) as response:
        if response.status != 200:
            raise RuntimeError(f"HTTP {response.status}")
        with tempfile.NamedTemporaryFile(
            mode="wb", delete=False, dir=destination.parent, suffix=".part"
        ) as temporary_file:
            temporary_path = Path(temporary_file.name)
            while chunk := response.read(1024 * 1024):
                temporary_file.write(chunk)
    os.replace(temporary_path, destination)


def main():
    args = parse_args()
    database = args.database.resolve()
    covers_dir = args.myfiles_dir.resolve() / "covers"

    if not database.is_file():
        print(f"Database not found: {database}", file=sys.stderr)
        return 2

    covers_dir.mkdir(parents=True, exist_ok=True)
    connection = sqlite3.connect(database)
    connection.row_factory = sqlite3.Row
    scenes = connection.execute(
        """
        SELECT id, scene_id, cover_url, images
        FROM scenes
        WHERE is_available = 1
          AND scene_id <> ''
          AND deleted_at IS NULL
        ORDER BY id
        """
    ).fetchall()

    downloaded = 0
    skipped = 0
    failed = 0
    failures = []

    try:
        for scene in scenes:
            scene_id = scene["scene_id"]
            cover_url = scene["cover_url"]
            destination = covers_dir / f"{scene_id}.jpg"

            if destination.is_file() and destination.stat().st_size > 0:
                local_cover_url = f"/myfiles/covers/{scene_id}.jpg"
                if cover_url != local_cover_url:
                    connection.execute(
                        "UPDATE scenes SET cover_url = ? WHERE id = ?",
                        (local_cover_url, scene["id"]),
                    )
                    connection.commit()
                skipped += 1
                continue

            download_url = cover_url
            parsed_url = urlparse(download_url)
            if parsed_url.scheme not in {"http", "https"}:
                download_url = get_first_image_url(scene["images"])
                if not download_url:
                    print(f"Skipping {scene_id}: no remote cover URL or image URL")
                    skipped += 1
                    continue
                parsed_url = urlparse(download_url)
            if parsed_url.scheme not in {"http", "https"}:
                print(f"Skipping {scene_id}: image URL is not remote: {download_url}")
                skipped += 1
                continue

            try:
                download_cover(download_url, destination, args.timeout, args.insecure)
                local_cover_url = f"/myfiles/covers/{scene_id}.jpg"
                connection.execute(
                    "UPDATE scenes SET cover_url = ? WHERE id = ?",
                    (local_cover_url, scene["id"]),
                )
                connection.commit()
                downloaded += 1
                print(f"Downloaded {scene_id}")
            except (HTTPError, URLError, InvalidURL, OSError, RuntimeError, ValueError) as error:
                failed += 1
                failures.append((scene_id, download_url, str(error)))
                print(f"Failed {scene_id}: {error}", file=sys.stderr)
    finally:
        connection.close()

    print(
        f"Completed: {downloaded} downloaded, {skipped} skipped, {failed} failed, "
        f"{len(scenes)} available scenes checked."
    )
    if failures:
        print("Failed cover downloads:")
        for scene_id, cover_url, error in failures:
            print(f"- {scene_id}: {error}\n  {cover_url}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
