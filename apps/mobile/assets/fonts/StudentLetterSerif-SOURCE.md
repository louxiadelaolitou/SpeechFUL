# SpeechFUL Letter Serif

`StudentLetterSerif.ttf` is a renamed, static weight-400 subset of Noto Serif SC,
distributed under the adjacent `OFL-NotoSerifSC.txt`. It contains the characters
used in `lib/src/localization/translations_student_letter.dart`, including all
four letter translations. Other application text keeps its existing fonts.

- Upstream: `https://github.com/google/fonts/tree/main/ofl/notoserifsc`
- Download: `https://raw.githubusercontent.com/google/fonts/main/ofl/notoserifsc/NotoSerifSC%5Bwght%5D.ttf`
- Original SHA256: `050080d9255a86808f2945bffac582b31ef32bc36411ce29563b4961670c66f9`
- Subset SHA256: `f9c5b3ce9f007fc1c0f0dc729dd767dfba8758f606376bc4d3ee98a07c5b0e6c`
- Subset size: 130,532 bytes.
- Toolchain: Python, fontTools 4.68.0.

From the repository root, with the original upstream font available locally:

```sh
python tools/subset-student-letter-font.py /path/to/NotoSerifSC-Variable.ttf
```

Rebuild the subset after changing or adding letter translations. The generator
checks upstream character coverage and changes the modified font's family and
PostScript names to SpeechFUL Letter Serif.
