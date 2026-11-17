"""Build the letter's static serif subset from the official OFL font."""

import argparse
import hashlib
import json
from pathlib import Path

from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    catalog = root / "apps/mobile/lib/src/localization/translations_student_letter.dart"
    output = root / "apps/mobile/assets/fonts/StudentLetterSerif.ttf"
    text = catalog.read_text(encoding="utf-8") + " "
    font = TTFont(args.source)
    missing = set(map(ord, text)) - set(font.getBestCmap()) - {10, 13}
    if missing:
        raise ValueError(f"Source font lacks code points: {sorted(missing)}")
    font = instantiateVariableFont(font, {"wght": 400}, inplace=True)
    options = subset.Options()
    options.recalc_timestamp = False
    subsetter = subset.Subsetter(options=options)
    subsetter.populate(text=text)
    subsetter.subset(font)
    # Modified fonts receive a distinct family and PostScript name.
    names = {
        1: "SpeechFUL Letter Serif",
        2: "Regular",
        3: "SpeechFUL-Letter-Serif-Regular-v1",
        4: "SpeechFUL Letter Serif Regular",
        6: "SpeechFUL-Letter-Serif-Regular",
        16: "SpeechFUL Letter Serif",
        17: "Regular",
    }
    for record in font["name"].names:
        if record.nameID in names:
            record.string = names[record.nameID].encode(record.getEncoding())
    font.save(output)
    print(json.dumps({
        "source_sha256": hashlib.sha256(args.source.read_bytes()).hexdigest(),
        "output_sha256": hashlib.sha256(output.read_bytes()).hexdigest(),
        "bytes": output.stat().st_size,
    }))


if __name__ == "__main__":
    main()
