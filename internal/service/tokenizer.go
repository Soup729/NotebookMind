package service

import (
	"strings"
	"unicode"

	"github.com/yanyiwu/gojieba"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ============ 统一分词器 ============

// Tokenizer 统一分词器：中英文混合分词 + 停用词过滤 + 词形归一化
type Tokenizer struct {
	jieba      *gojieba.Jieba
	stopwords  map[string]struct{}
	normalizer transform.Transformer
}

// NewTokenizer 创建统一分词器
// useDefaultDict: 是否使用 gojieba 内置词典（true 时 dictPath 等参数可为空）
func NewTokenizer() *Tokenizer {
	jieba := gojieba.NewJieba()

	stopwords := make(map[string]struct{}, len(defaultStopwords))
	for _, w := range defaultStopwords {
		stopwords[w] = struct{}{}
	}

	// 重音符剥离器: é → e, ü → u, etc.
	normalizer := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

	return &Tokenizer{
		jieba:      jieba,
		stopwords:  stopwords,
		normalizer: normalizer,
	}
}

// Free 释放 gojieba 资源
func (t *Tokenizer) Free() {
	if t.jieba != nil {
		t.jieba.Free()
	}
}

// Tokenize 分词 + 停用词过滤 + 归一化
func (t *Tokenizer) Tokenize(text string) []string {
	normalized := t.Normalize(text)
	tokens := t.jieba.CutForSearch(normalized, true)
	return t.filterStopwords(tokens)
}

// TokenizeForSearch 搜索模式分词（CutForSearch，更细粒度）
func (t *Tokenizer) TokenizeForSearch(text string) []string {
	normalized := t.Normalize(text)
	tokens := t.jieba.CutForSearch(normalized, true)
	return t.filterStopwords(tokens)
}

// Normalize 归一化：小写 + 剥离重音符 + 清理空白
func (t *Tokenizer) Normalize(text string) string {
	// 剥离重音符
	result, _, _ := transform.String(t.normalizer, text)
	// 转小写
	result = strings.ToLower(result)
	// 清理多余空白
	result = strings.TrimSpace(result)
	// 替换换行/制表符为空格
	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.ReplaceAll(result, "\t", " ")
	result = strings.ReplaceAll(result, "\r", " ")
	return result
}

// IsStopword O(1) 停用词检查
func (t *Tokenizer) IsStopword(word string) bool {
	_, ok := t.stopwords[word]
	return ok
}

// AddStopwords 添加自定义停用词
func (t *Tokenizer) AddStopwords(words []string) {
	for _, w := range words {
		t.stopwords[w] = struct{}{}
	}
}

// filterStopwords 过滤停用词和过短的 token
func (t *Tokenizer) filterStopwords(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		// 过滤停用词
		if _, ok := t.stopwords[token]; ok {
			continue
		}
		// 过滤单字符（中文单字保留，英文单字母过滤）
		if len(token) == 1 && token[0] < 128 {
			continue
		}
		result = append(result, token)
	}
	return result
}

// ============ 默认停用词表 ============

var defaultStopwords = []string{
	// 英文停用词
	"a", "an", "the", "and", "or", "but", "if", "then", "else",
	"is", "are", "was", "were", "be", "been", "being",
	"have", "has", "had", "do", "does", "did",
	"will", "would", "could", "should", "may", "might", "must", "can",
	"to", "of", "in", "for", "on", "with", "at", "by", "from", "as",
	"into", "through", "during", "before", "after", "above", "below",
	"between", "out", "off", "over", "under", "again", "further",
	"this", "that", "these", "those", "it", "its",
	"i", "me", "my", "myself", "we", "our", "ours",
	"you", "your", "yours", "he", "him", "his",
	"she", "her", "hers", "they", "them", "their",
	"what", "which", "who", "whom", "when", "where", "why", "how",
	"not", "no", "nor", "so", "very", "just", "about",
	"up", "down", "all", "each", "both", "few", "more", "most",
	"other", "some", "such", "only", "own", "same", "than",
	"too", "also",

	// 中文停用词
	"的", "了", "是", "在", "和", "有", "我", "你", "他", "她",
	"它", "们", "来", "去", "对", "这", "那", "就", "也", "都",
	"而", "及", "与", "或", "个", "为", "中", "以", "之", "上",
	"下", "不", "人", "一", "个", "会", "要", "说", "着", "没",
	"好", "把", "被", "从", "让", "到", "能", "还", "可以",
	"因为", "所以", "但是", "如果", "那么", "虽然", "然后",
	"什么", "怎么", "哪", "谁", "多少", "几", "怎样",
}
