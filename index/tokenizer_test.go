package index

import (
	"reflect"
	"testing"
)

func TestMixedScriptTokenizer_English(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("Go is a Fast language!")
	want := []string{"go", "is", "a", "fast", "language"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("English tokens: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_Digits(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("GPT-4 and Claude-3.5")
	want := []string{"gpt", "4", "and", "claude", "3", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Digits: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_ChineseBigram(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("知识库系统")
	want := []string{"知识", "识库", "库系", "系统"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chinese bigram: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_SingleChineseChar(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("用 Go 写")
	// "用" and "写" are single CJK runs (separated by ASCII), each emits a 1-gram.
	want := []string{"用", "go", "写"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Single CJK char: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_MixedChineseEnglish(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("OpenAI公司发布GPT-4")
	want := []string{"openai", "公司", "司发", "发布", "gpt", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mixed: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_PunctuationAndWhitespace(t *testing.T) {
	got := MixedScriptTokenizer{}.Tokenize("hello, 世界！hello")
	want := []string{"hello", "世界", "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Punctuation: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_OtherUnicodeLetters(t *testing.T) {
	// Latin extended (café), Greek, Cyrillic should all be word chars.
	got := MixedScriptTokenizer{}.Tokenize("café αβγ Привет")
	want := []string{"café", "αβγ", "привет"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Other Unicode: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_Japanese(t *testing.T) {
	// Hiragana run gets bigrammed alongside Han.
	got := MixedScriptTokenizer{}.Tokenize("こんにちは")
	want := []string{"こん", "んに", "にち", "ちは"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Hiragana: got %v, want %v", got, want)
	}
}

func TestMixedScriptTokenizer_Empty(t *testing.T) {
	if got := (MixedScriptTokenizer{}).Tokenize(""); len(got) != 0 {
		t.Errorf("empty input: got %v, want []", got)
	}
	if got := (MixedScriptTokenizer{}).Tokenize("   ,.!?"); len(got) != 0 {
		t.Errorf("punctuation only: got %v, want []", got)
	}
}

func TestTokenizerFunc(t *testing.T) {
	tk := TokenizerFunc(func(s string) []string { return []string{s} })
	got := tk.Tokenize("hello")
	if !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("TokenizerFunc adapter failed: %v", got)
	}
}
