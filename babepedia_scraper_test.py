#!/usr/bin/env python3
"""
Test script for Babepedia actor scraper.
Fetches a babe page and prints all fields that would be scraped,
matching the logic in model_external_reference.go.

Usage:
    python babepedia_scraper_test.py [babe_name]
    python babepedia_scraper_test.py Alexis_Crystal
    python babepedia_scraper_test.py Desiree_Nevada
"""

import sys
import re
import requests
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
    Parse measurements like '34C-24-34' (inches) or with spaces.
    Returns dict with band_size_cm, cup_size, waist_cm, hip_cm.
    """
    m = re.search(r'(\d{2,3})\s*([A-Za-z]{1,2})\s*[-–]\s*(\d{2,3})\s*[-–]\s*(\d{2,3})', text)
    if m:
        band_in = int(m.group(1))
        cup = m.group(2).upper()
        waist_in = int(m.group(3))
        hip_in = int(m.group(4))
        return {
            "band_size": round(band_in * 2.54),
            "cup_size": cup,
            "waist_size": round(waist_in * 2.54),
            "hip_size": round(hip_in * 2.54),
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

    # --- aliases ---
    aka_div = soup.select_one("div#aka")
    if aka_div:
        aka_links = aka_div.select("a")
        aliases = [a.text.strip() for a in aka_links if a.text.strip()]
        if aliases:
            result["aliases"] = ", ".join(aliases)

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
    aka_block = soup.select_one("div#aka-block") or soup.select_one("p#aka")
    print("\n--- RAW aka HTML ---")
    if aka_block:
        safe_print(aka_block, 500)

    # parse personal-info-block rows
    if personal_block:
        rows = personal_block.select("li, tr, div.info-row")
        print(f"\nFound {len(rows)} rows in personal-info-block")
        for row in rows:
            print(f"  ROW: {row.text.strip()[:100]}")

        # Try links inside personal block - babepedia uses <a> for each value
        all_links = personal_block.select("a")
        print(f"\nLinks in personal-info-block:")
        for a in all_links:
            href = a.get("href", "")
            text = a.text.strip()
            print(f"  [{text}] href={href}")
            # Determine field by href pattern
            if "birthday" in href or "born-in-the-year" in href:
                pass  # handled separately
            elif "topbabespercountry" in href:
                result["nationality"] = lookup_country(text)
            elif "topbabesperstate" in href and "nationality" not in result:
                pass  # state/city, skip
            elif any(x in href for x in ["caucasian", "asian", "latin", "ebony", "black"]):
                result["ethnicity"] = text
            elif any(x in href for x in ["hair"]):
                result["hair_color"] = text
            elif any(x in href for x in ["eyes", "brown", "blue", "green", "hazel"]):
                result["eye_color"] = text
            elif any(x in href for x in ["slim", "athletic", "curvy", "bbw", "petite"]):
                pass  # body type
            elif any(x in href for x in ["fake", "real", "natural", "enhanced", "breast"]):
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

    # image
    prof_img = soup.select_one("div#profimg img")
    if prof_img:
        src = prof_img.get("src") or prof_img.get("data-src", "")
        if src:
            result["image_url"] = src

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


if __name__ == "__main__":
    babe = sys.argv[1] if len(sys.argv) > 1 else "Alexis_Crystal"
    scrape_babepedia(babe)
    if len(sys.argv) > 2:
        scrape_babepedia(sys.argv[2])
