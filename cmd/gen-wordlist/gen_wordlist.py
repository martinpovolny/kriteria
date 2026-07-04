#!/usr/bin/env python3
"""
Generate adjective and noun wordlists using the MorphoDiTa morphological analyzer
(ÚFAL, Charles University). Fetches a Czech frequency list, batches words through
the MorphoDiTa REST API analyze endpoint, and filters lemmas by POS tag.

Outputs: adjectives.txt, nouns.txt (diacritics stripped, 4-10 chars, deduplicated)

Usage:
    python3 gen_wordlist.py [--max-words 50000] [--batch-size 200] [--output-dir ../../internal/api]
"""

import argparse
import json
import math
import subprocess
import sys
import time
import urllib.parse
from pathlib import Path

MORPHODITA_URL = "https://lindat.mff.cuni.cz/services/morphodita/api/analyze"
FREQ_LIST_URL = "https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018/cs/cs_50k.txt"

MIN_LEN = 4
MAX_LEN = 10


def strip_diacritics(s: str) -> str:
    """Remove Czech diacritics: á→a, č→c, ď→d, ě→e, í→i, ň→n, ó→o, ř→r, š→s, ť→t, ú→u, ů→u, ý→y, ž→z"""
    table = str.maketrans({
        "á": "a", "č": "c", "ď": "d", "ě": "e", "é": "e",
        "í": "i", "ň": "n", "ó": "o", "ř": "r", "š": "s",
        "ť": "t", "ú": "u", "ů": "u", "ý": "y", "ž": "z",
        "Á": "a", "Č": "c", "Ď": "d", "Ě": "e", "É": "e",
        "Í": "i", "Ň": "n", "Ó": "o", "Ř": "r", "Š": "s",
        "Ť": "t", "Ú": "u", "Ů": "u", "Ý": "y", "Ž": "z",
    })
    return s.translate(table)


def download_frequency_list(max_words: int) -> list[str]:
    """Download Czech frequency list from FrequencyWords (OpenSubtitles 2018)."""
    print(f"Downloading frequency list from {FREQ_LIST_URL} ...", file=sys.stderr)
    result = subprocess.run(
        ["curl", "-sL", FREQ_LIST_URL],
        capture_output=True, timeout=30,
    )
    if result.returncode != 0:
        raise RuntimeError(f"curl failed: {result.stderr.decode('utf-8', errors='replace')}")
    words = []
    for line in result.stdout.decode("utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split()
        if parts:
            words.append(parts[0])
        if len(words) >= max_words:
            break
    print(f"  got {len(words)} words", file=sys.stderr)
    return words


def analyze_batch(words: list[str]) -> list:
    """Send a batch of words to MorphoDiTa analyze API. Returns JSON result."""
    text = " ".join(words)
    params = urllib.parse.urlencode({"data": text, "output": "json", "guesser": "no"})
    url = MORPHODITA_URL + "?" + params

    for attempt in range(3):
        try:
            result = subprocess.run(
                ["curl", "-sL", "--max-time", "60", url],
                capture_output=True, timeout=70,
            )
            if result.returncode != 0:
                raise RuntimeError(f"curl exit {result.returncode}")
            data = json.loads(result.stdout.decode("utf-8"))
            return data.get("result", [])
        except Exception as e:
            if attempt < 2:
                print(f"  retry {attempt+1}/3: {e}", file=sys.stderr)
                time.sleep(2 ** attempt)
            else:
                raise

    return []


def classify_lemmas(analyses_result: list, nouns: dict, adjectives: dict):
    """Extract noun and adjective lemmas from analyze result.

    POS tag position 0: N=noun, A=adjective
    We collect unique lemmas (base forms) per POS.

    Tag format (positions):
      0: POS (N, A, V, C, D, J, P, R, T, X, F, B, I)
      1: subPOS (gender for nouns: M, F, N)
      2: gender detail (I animate / I inanimate for masc)
      3: number (S=singular, P=plural)
      4: case (1-7)
    """
    for sentence in analyses_result:
        for token in sentence:
            word = token.get("token", "")
            for analysis in token.get("analyses", []):
                tag = analysis.get("tag", "")
                lemma = analysis.get("lemma", "")
                if not tag or not lemma:
                    continue

                pos = tag[0]

                # Clean lemma: strip morphological comments after semicolon/underscore
                clean_lemma = lemma.split(";")[0].split("_")[0].split("-")[0].strip()

                if not clean_lemma:
                    continue

                if pos == "N":
                    nouns[clean_lemma] = tag
                elif pos == "A":
                    adjectives[clean_lemma] = tag


def filter_word(word: str) -> str | None:
    """Filter a single word: strip diacritics, check length, return ASCII form or None."""
    ascii_word = strip_diacritics(word.lower())

    if len(ascii_word) < MIN_LEN or len(ascii_word) > MAX_LEN:
        return None

    # Only letters, no numbers or special chars
    if not ascii_word.isalpha():
        return None

    return ascii_word


def main():
    parser = argparse.ArgumentParser(description="Generate adjective+noun wordlists via MorphoDiTa")
    parser.add_argument("--max-words", type=int, default=50000,
                        help="Max words from frequency list (default: 50000)")
    parser.add_argument("--batch-size", type=int, default=200,
                        help="Words per API request (default: 200)")
    parser.add_argument("--output-dir", type=str, default="../../internal/api",
                        help="Output directory for adjectives.txt and nouns.txt")
    args = parser.parse_args()

    words = download_frequency_list(args.max_words)

    nouns: dict[str, str] = {}           # lemma → tag
    adjectives: dict[str, str] = {}      # lemma → tag

    total_batches = math.ceil(len(words) / args.batch_size)
    for i in range(0, len(words), args.batch_size):
        batch = words[i:i + args.batch_size]
        batch_num = i // args.batch_size + 1
        print(f"  analyzing batch {batch_num}/{total_batches} ({len(batch)} words)...",
              file=sys.stderr, end="", flush=True)

        result = analyze_batch(batch)
        classify_lemmas(result, nouns, adjectives)
        print(f"  nouns={len(nouns)} adj={len(adjectives)}", file=sys.stderr)

        # Be gentle with the API
        if i + args.batch_size < len(words):
            time.sleep(0.5)

    print(f"\nRaw lemmas: {len(nouns)} nouns, {len(adjectives)} adjectives", file=sys.stderr)

    # Filter and deduplicate
    noun_words = set()
    for lemma in nouns:
        w = filter_word(lemma)
        if w:
            noun_words.add(w)

    adj_words = set()
    for lemma in adjectives:
        w = filter_word(lemma)
        if w:
            adj_words.add(w)

    # Remove words that appear in both lists (ambiguous)
    common = noun_words & adj_words
    if common:
        print(f"  removing {len(common)} ambiguous words (appear in both lists)", file=sys.stderr)
        noun_words -= common
        adj_words -= common

    noun_list = sorted(noun_words)
    adj_list = sorted(adj_words)

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    nouns_path = output_dir / "nouns.txt"
    adjs_path = output_dir / "adjectives.txt"

    nouns_path.write_text("\n".join(noun_list) + "\n", encoding="utf-8")
    adjs_path.write_text("\n".join(adj_list) + "\n", encoding="utf-8")

    print(f"\nOutput:", file=sys.stderr)
    print(f"  {nouns_path} — {len(noun_list)} nouns", file=sys.stderr)
    print(f"  {adjs_path} — {len(adj_list)} adjectives", file=sys.stderr)

    # Print sample
    print("\nSample nouns:", file=sys.stderr)
    for w in noun_list[:10]:
        print(f"  {w}", file=sys.stderr)
    print("\nSample adjectives:", file=sys.stderr)
    for w in adj_list[:10]:
        print(f"  {w}", file=sys.stderr)


if __name__ == "__main__":
    main()
