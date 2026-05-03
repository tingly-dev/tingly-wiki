package index

import (
	"strings"
	"unicode"
)

// Tokenizer splits text into search tokens. Implementations decide how to
// handle scripts, casing, normalization, and segmentation. The same Tokenizer
// must be used for both indexing and querying or recall will be poor.
type Tokenizer interface {
	Tokenize(text string) []string
}

// TokenizerFunc adapts a plain function into a Tokenizer.
type TokenizerFunc func(text string) []string

// Tokenize implements Tokenizer.
func (f TokenizerFunc) Tokenize(text string) []string { return f(text) }

// MixedScriptTokenizer is a zero-dependency tokenizer that handles ASCII,
// CJK, and other Unicode scripts in a single pass.
//
// Behavior:
//   - ASCII letters/digits: lowercased, split on any non-word rune (matches
//     the prior English-only tokenizer for backward compatibility).
//   - Han / Hiragana / Katakana / Hangul runs: emitted as bigrams. A run of
//     length 1 becomes a single 1-gram. Bigrams give reasonable BM25 recall
//     without a CJK dictionary.
//   - Other Unicode letters (Greek, Cyrillic, Arabic, etc.): treated as word
//     characters and split on non-letter runes; lowercased where applicable.
//   - Everything else (punctuation, whitespace, symbols): acts as a delimiter.
type MixedScriptTokenizer struct{}

// Tokenize implements Tokenizer.
func (MixedScriptTokenizer) Tokenize(text string) []string {
	if text == "" {
		return nil
	}

	var tokens []string
	var word strings.Builder
	var cjk []rune

	flushWord := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		if len(cjk) == 1 {
			tokens = append(tokens, string(cjk))
		} else {
			for i := 0; i < len(cjk)-1; i++ {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case isWordRune(r):
			flushCJK()
			word.WriteRune(unicode.ToLower(r))
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()

	return tokens
}

// isCJK reports whether r belongs to a script we segment by bigram.
func isCJK(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r):
		return true
	case unicode.Is(unicode.Hiragana, r):
		return true
	case unicode.Is(unicode.Katakana, r):
		return true
	case unicode.Is(unicode.Hangul, r):
		return true
	}
	return false
}

// isWordRune reports whether r should be folded into a contiguous word.
// Letters and digits across all scripts qualify (after CJK is handled separately).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// DefaultTokenizer returns the default Tokenizer used by FullTextIndex.
func DefaultTokenizer() Tokenizer { return MixedScriptTokenizer{} }
