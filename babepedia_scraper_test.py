#!/usr/bin/env python3
"""
Test script for Babepedia actor scraper.
Fetches a babe page, prints scraped fields, and optionally updates main.db.

Usage:
    # Dry-run (print only, no DB changes):
    python babepedia_scraper_test.py Desiree_Nevada
    python babepedia_scraper_test.py Alexis_Crystal

    # Update DB for a specific actor by babepedia name:
    python babepedia_scraper_test.py Desiree_Nevada --update

    # Update DB for ALL actors that have a babepedia scrape URL in their urls field:
    python babepedia_scraper_test.py --update-all

DB path defaults to G:/USD_XBVR/xbvr/main.db (dollar sign in path), override with --db PATH
"""

import sys
import re
import requests
from urllib.parse import quote
from bs4 import BeautifulSoup

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                  "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
}

CUP_ORDER = ["AA", "A", "B", "C", "D", "DD", "E", "F", "G", "H", "I", "J", "K"]

COUNTRY_MAP = {
    "United States": "US", "USA": "US",
    "Czech Republic": "CZ", "Czechia": "CZ",
    "Russia": "RU", "Ukraine": "UA",
    "Germany": "DE", "France": "FR", "Italy": "IT",
    "Spain": "ES", "Hungary": "HU", "Poland": "PL",
    "Brazil": "BR", "Colombia": "CO", "Romania": "RO",
    "United Kingdom": "GB", "UK": "GB",
    "Japan": "JP", "China": "CN",
    "Australia": "AU", "Canada": "CA",
}


def inch_to_cm(val_str):
    """Convert inch string to cm int."""
    try:
        return round(int(val_str) * 2.54)
    except Exception:
        return None


def lookup_country(name):
    return COUNTRY_MAP.get(name.strip(), name.strip())


def parse_measurements(text):
    """
    Parse measurements like '34C-24-34' or '32-22-31' (inches).
    Returns dict with band_size, cup_size, waist_size, hip_size.
    """
    # with cup letter: 34C-24-34
    m = re.search(r'(\d{2,3})\s*([A-Za-z]{1,2})\s*[-–]\s*(\d{2,3})\s*[-–]\s*(\d{2,3})', text)
    if m:
        return {
            "band_size": round(int(m.group(1)) * 2.54),
            "cup_size": m.group(2).upper(),
            "waist_size": round(int(m.group(3)) * 2.54),
            "hip_size": round(int(m.group(4)) * 2.54),
        }
    # numeric only: 32-22-31
    m = re.search(r'(\d{2,3})\s*[-–]\s*(\d{2,3})\s*[-–]\s*(\d{2,3})', text)
    if m:
        return {
            "band_size": round(int(m.group(1)) * 2.54),
            "waist_size": round(int(m.group(2)) * 2.54),
            "hip_size": round(int(m.group(3)) * 2.54),
        }
    return None


def scrape_babepedia(babe_name):
    url = f"https://www.babepedia.com/babe/{babe_name}"
    print(f"\n{'='*60}")
    print(f"Scraping: {url}")
    print('='*60)

    resp = requests.get(url, headers=HEADERS, timeout=15)
    if resp.status_code != 200:
        print(f"ERROR: HTTP {resp.status_code}")
        return

    soup = BeautifulSoup(resp.text, "html.parser")

    result = {}

    # --- image_url: profile photo ---
    img = soup.select_one("img#babepic")
    if not img:
        img = soup.select_one("div#bioPic img")
    if not img:
        # try og:image
        og = soup.find("meta", property="og:image")
        if og:
            result["image_url"] = og.get("content", "")
    if img:
        result["image_url"] = img.get("src") or img.get("data-src", "")

    # --- aliases: h2#aka contains "Also known as: Name1 - Name2 - ..." as plain text ---
    aliases = []
    aka_h2 = soup.select_one("h2#aka")
    if aka_h2:
        # remove child elements (small label, img), keep only text nodes
        text = aka_h2.get_text(" ", strip=True)
        text = re.sub(r'also known as[:\s]*', '', text, flags=re.I).strip()
        parts = [p.strip() for p in re.split(r'\s*-\s*', text) if p.strip()]
        aliases = parts
    if not aliases:
        # fallback: links inside div#aka / div#aka-block
        for sel in ["div#aka a", "div#aka-block a", "div#alsoknown a", "p#aka a"]:
            links = soup.select(sel)
            if links:
                aliases = [a.text.strip() for a in links if a.text.strip()]
                break
    if aliases:
        result["aliases"] = aliases

    # --- biography ---
    bio_div = soup.select_one("div#about p") or soup.select_one("div.about p")
    if bio_div:
        result["biography"] = bio_div.text.strip()

    # --- Parse #personal-info-block (actual babepedia structure) ---
    personal_block = soup.select_one("div#personal-info-block")

    def safe_print(s, limit=3000):
        print(str(s)[:limit].encode("ascii", "replace").decode("ascii"))

    print("\n--- RAW #personal-info-block HTML ---")
    if personal_block:
        safe_print(personal_block, 3000)
    else:
        print("NOT FOUND")

    # cupconversions block (hidden, contains cm values)
    cup_block = soup.select_one("span#cupconversions")
    print("\n--- RAW #cupconversions HTML ---")
    if cup_block:
        safe_print(cup_block, 1000)
    else:
        print("NOT FOUND")

    # aliases
    print("\n--- RAW aka HTML ---")
    aka_el = soup.select_one("h2#aka")
    if aka_el:
        safe_print(aka_el, 500)

    # parse personal-info-block rows
    if personal_block:
        rows = personal_block.select("li, tr, div.info-row")
        print(f"\nFound {len(rows)} rows in personal-info-block")
        for row in rows:
            print(f"  ROW: {row.text.strip()[:100]}")

        # Parse structured div.info-item blocks (new babepedia layout)
        info_items = personal_block.select("div.info-item")
        print(f"\nFound {len(info_items)} div.info-item blocks")
        for item in info_items:
            label_el = item.select_one("span.label")
            value_el = item.select_one("span.value")
            if not label_el or not value_el:
                continue
            label = label_el.text.strip().rstrip(":").lower()
            value_text = value_el.text.strip()
            # also get first link text and href
            first_link = value_el.select_one("a")
            link_href = first_link.get("href", "") if first_link else ""
            link_text = first_link.text.strip() if first_link else ""
            print(f"  [{label}] => [{value_text[:80]}]")

            if "hair" in label:
                result["hair_color"] = link_text or value_text
            elif "eye" in label:
                result["eye_color"] = link_text or value_text
            elif "height" in label:
                m = re.search(r'(\d{2,3})\s*cm', value_text)
                if m:
                    result["height"] = int(m.group(1))
                else:
                    m = re.search(r"(\d+)'(\d+)", value_text)
                    if m:
                        result["height"] = round(int(m.group(1)) * 30.48 + int(m.group(2)) * 2.54)
            elif "weight" in label:
                m = re.search(r'(\d{2,3})\s*kg', value_text)
                if m:
                    result["weight"] = int(m.group(1))
                else:
                    m = re.search(r'(\d{2,3})\s*lb', value_text)
                    if m:
                        result["weight"] = round(int(m.group(1)) * 0.453592)
            elif "bra" in label or "cup" in label:
                # e.g. "36D" -> band_size=36 inches, cup_size=D
                m = re.match(r'(\d{2,3})([A-Za-z]{1,2})', value_text.strip())
                if m:
                    result["band_size"] = inch_to_cm(m.group(1))
                    result["cup_size"] = m.group(2).upper()
            elif "years active" in label or "career" in label:
                # e.g. "2020-2022 (started around 23 years old; 2 years active)"
                m = re.search(r'(\d{4})[^\d]*(\d{4})?', value_text)
                if m:
                    result["start_year"] = int(m.group(1))
                    if m.group(2):
                        result["end_year"] = int(m.group(2))
            elif "ethnic" in label:
                result["ethnicity"] = link_text or value_text
            elif "tattoo" in label and value_text.lower() not in ("no", "none", ""):
                # store as JSON array: split on semicolons
                items = [t.strip() for t in value_text.split(";") if t.strip()]
                result["tattoos"] = items
            elif "piercing" in label and value_text.lower() not in ("no", "none", ""):
                items = [t.strip() for t in value_text.split(";") if t.strip()]
                result["piercings"] = items
            elif "boob" in label or ("breast" in label and "type" not in label):
                result["breast_type"] = link_text or value_text
            elif "measurement" in label:
                parsed = parse_measurements(value_text)
                if parsed:
                    for k, v in parsed.items():
                        if k not in result:
                            result[k] = v
            elif "also known" in label or "known as" in label:
                # e.g. "Candy Licious - Candee L - Sofia"
                parts = [p.strip() for p in re.split(r'\s*[-,]\s*', value_text) if p.strip()]
                if parts:
                    result["aliases"] = parts

        # Try links inside personal block for fields not in info-items
        all_links = personal_block.select("a")
        print(f"\nLinks in personal-info-block:")
        for a in all_links:
            href = a.get("href", "")
            text = a.text.strip()
            print(f"  [{text}] href={href}")
            if "topbabespercountry" in href:
                result["nationality"] = lookup_country(text)
            elif any(x in href for x in ["fake", "realbreast", "naturalbreast", "enhancedbreast",
                                          "top100fakebreasts", "top100realbreasts"]):
                if "breast_type" not in result:
                    result["breast_type"] = text

        # born date: two links - day/month + year
        birthday_links = personal_block.select("a[href*='birthday']")
        year_links = personal_block.select("a[href*='born-in-the-year']")
        if birthday_links and year_links:
            bday = birthday_links[0].text.strip()  # e.g. "1st of January"
            year = year_links[0].text.strip()       # e.g. "1997"
            result["birth_date_raw"] = f"{bday} {year}"

    # parse cupconversions for measurements & body stats
    if cup_block:
        cup_text = cup_block.get_text(" ", strip=True)
        print(f"\ncupconversions text: {cup_text}")
        # look for height in cm
        m = re.search(r'(\d{2,3})\s*cm', cup_text)
        if m and "height" not in result:
            result["height"] = int(m.group(1))
        # weight in kg
        m = re.search(r'(\d{2,3})\s*kg', cup_text)
        if m and "weight" not in result:
            result["weight"] = int(m.group(1))
        # measurements like 34C-24-34
        parsed = parse_measurements(cup_text)
        if parsed:
            for k, v in parsed.items():
                if k not in result:
                    result[k] = v

    # biography
    bio_p = soup.select_one("p#biotext")
    if bio_p:
        result["biography"] = bio_p.text.strip()

    def make_abs_encoded(href):
        """Make absolute URL (spaces kept as-is, Vue encodeURI handles encoding)."""
        if not href:
            return href
        if href.startswith("/"):
            href = "https://www.babepedia.com" + href
        return href

    # profile photo: use full-size href from profbox2 first link, fallback to img src
    IMAGE_EXTS = (".jpg", ".jpeg", ".png", ".webp", ".gif")
    prof_link = soup.select_one("div#profbox2 a.img")
    if prof_link:
        href = prof_link.get("href", "")
        if href and any(href.lower().endswith(ext) for ext in IMAGE_EXTS) and "_thumb" not in href:
            result["image_url"] = make_abs_encoded(href)
    if "image_url" not in result:
        prof_img = soup.select_one("div#profimg img")
        if prof_img:
            src = prof_img.get("src") or prof_img.get("data-src", "")
            if src:
                result["image_url"] = make_abs_encoded(src)

    # gallery images: full-size hrefs from user-uploads gallery only
    # profbox2 is excluded (contains _thumb pics and uploadphotos page links)
    IMAGE_EXTS = (".jpg", ".jpeg", ".png", ".webp", ".gif")
    all_images = []
    for a in soup.select("div.gallery.useruploads2 a.img"):
        href = a.get("href", "")
        if not href:
            continue
        # skip thumbnails and non-image links
        if "_thumb" in href:
            continue
        if not any(href.lower().endswith(ext) for ext in IMAGE_EXTS):
            continue
        href = make_abs_encoded(href)
        if href not in all_images:
            all_images.append(href)
    if all_images:
        result["extra_images"] = all_images
        print(f"\nFound {len(all_images)} gallery images: {all_images[:3]}")

    print("\n--- SCRAPED RESULT ---")
    for k, v in result.items():
        print(f"  {k:20s}: {v}")

    print("\n--- SUGGESTED Go scraper rules ---")
    print_go_rules(result, url)

    return result


def print_go_rules(result, url):
    """Print suggested Go scraper rule additions."""
    domain = "www.babepedia.com"
    print(f'''
// Add to buildGenericActorScraperRules() in model_external_reference.go:

siteDetails = GenericScraperRuleSet{{}}
siteDetails.Domain = "{domain}"
// NOTE: Selectors below are ESTIMATES - verify against actual HTML
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "image_url", Selector: `img#babepic`, ResultType: "attr", Attribute: "src"}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "aliases", Selector: `div#aka a`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "biography", Selector: `div#about p`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "hair_color", Selector: `ul#biodata li:contains("Hair") span.data`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "eye_color", Selector: `ul#biodata li:contains("Eye") span.data`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "height", Selector: `ul#biodata li:contains("Height") span.data`, PostProcessing: []PostProcessing{{{{Function: "RegexString", Params: []string{{`(\\d{{2,3}}) cm`, "1"}}}}}}}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "weight", Selector: `ul#biodata li:contains("Weight") span.data`, PostProcessing: []PostProcessing{{{{Function: "RegexString", Params: []string{{`(\\d{{2,3}}) kg`, "1"}}}}}}}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "nationality", Selector: `ul#biodata li:contains("Country") a`, PostProcessing: []PostProcessing{{{{Function: "Lookup Country"}}}}}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "ethnicity", Selector: `ul#biodata li:contains("Ethnic") span.data`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{XbvrField: "breast_type", Selector: `ul#biodata li:contains("Breast") span.data`}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{
    XbvrField: "band_size", Selector: `ul#biodata li:contains("Measurements") span.data`,
    PostProcessing: []PostProcessing{{
        {{Function: "RegexString", Params: []string{{`(\\d{{2,3}}).{{1,2}}-\\d{{2,3}}-\\d{{2,3}}`, "1"}}}},
        {{Function: "inch to cm"}},
    }},
}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{
    XbvrField: "cup_size", Selector: `ul#biodata li:contains("Measurements") span.data`,
    PostProcessing: []PostProcessing{{{{Function: "RegexString", Params: []string{{`\\d{{2,3}}(.{{1,2}})-\\d{{2,3}}-\\d{{2,3}}`, "1"}}}}}},
}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{
    XbvrField: "waist_size", Selector: `ul#biodata li:contains("Measurements") span.data`,
    PostProcessing: []PostProcessing{{
        {{Function: "RegexString", Params: []string{{`\\d{{2,3}}.{{1,2}}-(\\d{{2,3}})-\\d{{2,3}}`, "1"}}}},
        {{Function: "inch to cm"}},
    }},
}})
siteDetails.SiteRules = append(siteDetails.SiteRules, GenericActorScraperRule{{
    XbvrField: "hip_size", Selector: `ul#biodata li:contains("Measurements") span.data`,
    PostProcessing: []PostProcessing{{
        {{Function: "RegexString", Params: []string{{`\\d{{2,3}}.{{1,2}}-\\d{{2,3}}-(\\d{{2,3}})`, "1"}}}},
        {{Function: "inch to cm"}},
    }},
}})
scrapeRules.GenericActorScrapingConfig["babepedia scrape"] = siteDetails
''')


# ── DB field mapping ──────────────────────────────────────────────────────────
# Maps scraped dict keys to actors table columns.
# Only non-empty values will be written. Existing values are NOT overwritten
# unless --overwrite flag is set.
DB_FIELD_MAP = {
    "image_url":   "image_url",
    "biography":   "biography",
    "nationality": "nationality",
    "ethnicity":   "ethnicity",
    "hair_color":  "hair_color",
    "eye_color":   "eye_color",
    "breast_type": "breast_type",
    "aliases_add": "aliases",   # special: merge into JSON array
}


def update_actor_db(db_path, actor_id, actor_name, scraped, overwrite=False, dry_run=False):
    """Apply scraped fields to the actors table row."""
    import sqlite3, json, datetime

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cur = conn.cursor()

    cur.execute("SELECT * FROM actors WHERE id = ?", (actor_id,))
    row = cur.fetchone()
    if not row:
        print(f"  [DB] Actor id={actor_id} not found")
        conn.close()
        return

    updates = {}

    # Simple string fields
    for scraped_key, col in [
        ("image_url",   "image_url"),
        ("biography",   "biography"),
        ("nationality", "nationality"),
        ("ethnicity",   "ethnicity"),
        ("hair_color",  "hair_color"),
        ("eye_color",   "eye_color"),
        ("breast_type", "breast_type"),
        ("cup_size",    "cup_size"),
    ]:
        val = scraped.get(scraped_key, "")
        if not val:
            continue
        existing = row[col] or ""
        if existing and not overwrite:
            print(f"  [SKIP] {col}: already has '{existing[:60]}'")
        else:
            updates[col] = val
            print(f"  [SET]  {col}: '{str(val)[:80]}'")

    # JSON array fields: tattoos, piercings, aliases - stored as ["item1","item2"]
    for scraped_key, col in [("tattoos", "tattoos"), ("piercings", "piercings"), ("aliases", "aliases")]:
        val = scraped.get(scraped_key)  # list or None
        if not val:
            continue
        existing_raw = row[col] or ""
        try:
            existing_list = json.loads(existing_raw) if existing_raw else []
            if not isinstance(existing_list, list):
                existing_list = [existing_raw] if existing_raw else []
        except Exception:
            existing_list = [existing_raw] if existing_raw else []
        if existing_list and not overwrite:
            print(f"  [SKIP] {col}: already has {existing_list[:3]}")
        else:
            updates[col] = json.dumps(val)
            print(f"  [SET]  {col}: {val}")

    # Integer fields
    for scraped_key, col in [
        ("height",     "height"),
        ("weight",     "weight"),
        ("band_size",  "band_size"),
        ("waist_size", "waist_size"),
        ("hip_size",   "hip_size"),
        ("start_year", "start_year"),
        ("end_year",   "end_year"),
    ]:
        val = scraped.get(scraped_key)
        if not val:
            continue
        existing = row[col]
        if existing and not overwrite:
            print(f"  [SKIP] {col}: already has '{existing}'")
        else:
            updates[col] = val
            print(f"  [SET]  {col}: {val}")

    # birth_date: only update if current is zero/empty
    birth_raw = scraped.get("birth_date_raw", "")
    if birth_raw:
        existing_bd = row["birth_date"] or ""
        is_zero = not existing_bd or existing_bd.startswith("0001")
        if is_zero or overwrite:
            # parse full date: "4th of March 1995" or "January 1st 1997"
            bd_val = None
            # strip ordinal suffixes: 1st->1, 2nd->2, etc.
            clean = re.sub(r'(\d+)(st|nd|rd|th)', r'\1', birth_raw, flags=re.I)
            for fmt in ("%d of %B %Y", "%B %d %Y", "%d %B %Y"):
                try:
                    from datetime import datetime
                    dt = datetime.strptime(clean.strip(), fmt)
                    bd_val = dt.strftime("%Y-%m-%d") + " 00:00:00+00:00"
                    break
                except ValueError:
                    pass
            if not bd_val:
                # fallback: year only
                m = re.search(r'(\d{4})', birth_raw)
                if m:
                    bd_val = f"{m.group(1)}-01-01 00:00:00+00:00"
            if bd_val:
                updates["birth_date"] = bd_val
                print(f"  [SET]  birth_date: {bd_val}")
        else:
            print(f"  [SKIP] birth_date: already has '{existing_bd}'")

    # image_arr: add profile image + all gallery images to existing JSON array
    try:
        arr = json.loads(row["image_arr"] or "[]")
    except Exception:
        arr = []
    # clean up bad URLs; also decode any legacy %20/%2520 back to spaces
    original_arr = list(arr)
    BAD_PATTERNS = ("_thumb", "/uploadphotos/", "/user-uploads-thumbs/")
    cleaned = [u.replace("%2520", " ").replace("%20", " ") for u in arr if not any(p in u for p in BAD_PATTERNS)]
    removed = len(arr) - len([u for u in arr if not any(p in u for p in BAD_PATTERNS)])
    if removed:
        print(f"  [CLEAN] Removed {removed} bad URLs from image_arr")
    arr = cleaned
    added_images = []
    all_new_images = []
    main_img = scraped.get("image_url", "")
    if main_img:
        all_new_images.append(main_img)
    all_new_images.extend(scraped.get("extra_images", []))
    for img in all_new_images:
        if img and img not in arr:
            arr.append(img)
            added_images.append(img)
    if added_images or arr != original_arr:
        updates["image_arr"] = json.dumps(arr)
        if added_images:
            print(f"  [ADD]  image_arr: +{len(added_images)} images")
            for img in added_images:
                print(f"           {img}")
        else:
            print(f"  [UPDATE] image_arr: cleaned bad URLs, no new images")
    else:
        print(f"  [SKIP] image_arr: all images already present")

    if not updates:
        print("  [DB] No fields to update")
        conn.close()
        return

    updates["updated_at"] = datetime.datetime.now(datetime.timezone.utc).isoformat()

    if dry_run:
        print(f"  [DRY-RUN] Would update {list(updates.keys())}")
        conn.close()
        return

    set_clause = ", ".join(f"{k} = ?" for k in updates)
    values = list(updates.values()) + [actor_id]
    cur.execute(f"UPDATE actors SET {set_clause} WHERE id = ?", values)
    conn.commit()
    print(f"  [DB] Updated actor id={actor_id} ({actor_name})")
    conn.close()


def get_babepedia_actors_from_db(db_path):
    """Return list of (id, name, babepedia_url) for actors with babepedia scrape URL."""
    import sqlite3, json
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cur = conn.cursor()
    cur.execute("SELECT id, name, urls FROM actors WHERE urls LIKE '%babepedia%'")
    rows = cur.fetchall()
    conn.close()
    result = []
    for row in rows:
        try:
            urls = json.loads(row["urls"] or "[]")
        except Exception:
            continue
        for entry in urls:
            if isinstance(entry, dict) and "babepedia" in entry.get("type", ""):
                result.append((row["id"], row["name"], entry["url"]))
                break
    return result


def babe_name_from_url(url):
    """Extract babe name from babepedia URL."""
    m = re.search(r'/babe/(.+)$', url)
    return m.group(1) if m else None


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Babepedia scraper test / DB updater")
    parser.add_argument("babe", nargs="?", help="Babepedia babe name (e.g. Desiree_Nevada)")
    parser.add_argument("--update", action="store_true", help="Update matching actor in DB")
    parser.add_argument("--update-all", action="store_true", help="Update all actors in DB that have babepedia URL")
    parser.add_argument("--overwrite", action="store_true", help="Overwrite existing non-empty fields")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be updated without writing")
    parser.add_argument("--db", default="G:/$XBVR/xbvr/main.db".replace("$", "$"), help="Path to main.db")
    args = parser.parse_args()

    if args.update_all:
        import sqlite3
        print(f"Scanning DB: {args.db}")
        actors = get_babepedia_actors_from_db(args.db)
        print(f"Found {len(actors)} actors with babepedia scrape URL")
        for actor_id, actor_name, bp_url in actors:
            babe = babe_name_from_url(bp_url)
            if not babe:
                print(f"  [SKIP] Cannot parse babe name from {bp_url}")
                continue
            print(f"\n{'='*60}\n{actor_name} (id={actor_id}) -> {bp_url}")
            scraped = scrape_babepedia(babe)
            if scraped:
                update_actor_db(args.db, actor_id, actor_name, scraped,
                                overwrite=args.overwrite, dry_run=args.dry_run)

    elif args.babe:
        scraped = scrape_babepedia(args.babe)
        if scraped and args.update:
            import sqlite3
            # find actor by name derived from babe slug
            actor_name_guess = args.babe.replace("_", " ")
            conn = sqlite3.connect(args.db)
            conn.row_factory = sqlite3.Row
            cur = conn.cursor()
            cur.execute("SELECT id, name FROM actors WHERE name = ? COLLATE NOCASE", (actor_name_guess,))
            row = cur.fetchone()
            conn.close()
            if row:
                print(f"\nFound actor: id={row['id']} name={row['name']}")
                update_actor_db(args.db, row["id"], row["name"], scraped,
                                overwrite=args.overwrite, dry_run=args.dry_run)
            else:
                print(f"\n[ERROR] Actor '{actor_name_guess}' not found in DB.")
                print("Tip: check spelling or add babepedia URL to actor manually.")
    else:
        parser.print_help()
