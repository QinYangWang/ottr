package asr

import (
	"errors"
	"strings"
	"unicode"
)

// DefaultLanguage is what a transcript says when the text offers no evidence:
// the acronym for an undetermined language.
const DefaultLanguage = "und"

// DetectLanguage guesses the spoken language from the text a recognizer
// produced, by the script it used: kana means Japanese, hangul Korean, Han
// Chinese, and Latin letters a Latin-script language (English here, which is
// what the recognizer writes). It returns DefaultLanguage when there is
// nothing to judge by.
//
// shortcut: the script only, not language identification. Upgrade when an
// sherpa-onnx language-ID model is bundled, or the user says otherwise.
func DetectLanguage(segments []Segment) string {
	var han, kana, hangul, latin int
	for _, seg := range segments {
		for _, r := range seg.Text {
			switch {
			case unicode.In(r, unicode.Hiragana, unicode.Katakana):
				kana++
			case unicode.Is(unicode.Hangul, r):
				hangul++
			case unicode.Is(unicode.Han, r):
				han++
			case unicode.Is(unicode.Latin, r):
				latin++
			}
		}
	}
	switch {
	case kana > 0:
		return "ja"
	case hangul > 0:
		return "ko"
	case han > latin:
		return "zh"
	case latin > 0:
		return "en"
	}
	return DefaultLanguage
}

// SetLanguage records the language the user says a transcript is in, which
// is what translation takes as its source.
func (s *Service) SetLanguage(mediaID int64, lang string) error {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = DefaultLanguage
	}
	db, err := s.store()
	if err != nil {
		return err
	}
	res, err := db.Exec(`UPDATE transcript SET language = ? WHERE media_id = ?`, lang, mediaID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errors.New("no transcript yet to set a language on")
	}
	return nil
}
