package provider

import (
	"net/url"
	"strings"
)

// commonWords are English function words. They stay in words.json and are
// left out of clouds and trends unless the reader asks for them.
var commonWords = map[string]bool{
	"a": true, "about": true, "above": true, "after": true, "again": true, "against": true,
	"all": true, "am": true, "an": true, "and": true, "any": true, "are": true, "aren't": true,
	"as": true, "at": true, "be": true, "because": true, "been": true, "before": true,
	"being": true, "below": true, "between": true, "both": true, "but": true, "by": true,
	"can": true, "can't": true, "cannot": true, "could": true, "couldn't": true,
	"did": true, "didn't": true, "do": true, "does": true, "doesn't": true, "doing": true, "don't": true,
	"down": true, "during": true, "each": true, "few": true, "for": true, "from": true, "further": true,
	"had": true, "hadn't": true, "has": true, "hasn't": true, "have": true, "haven't": true, "having": true,
	"he": true, "he'd": true, "he'll": true, "he's": true, "her": true, "here": true, "here's": true,
	"hers": true, "herself": true, "him": true, "himself": true, "his": true, "how": true, "how's": true,
	"i": true, "i'd": true, "i'll": true, "i'm": true, "i've": true, "if": true, "in": true, "into": true,
	"is": true, "isn't": true, "it": true, "it's": true, "its": true, "itself": true,
	"let's": true, "me": true, "more": true, "most": true, "my": true, "myself": true,
	"no": true, "nor": true, "not": true, "of": true, "off": true, "on": true, "once": true, "only": true,
	"or": true, "other": true, "ought": true, "our": true, "ours": true, "ourselves": true,
	"out": true, "over": true, "own": true, "same": true,
	"she": true, "she'd": true, "she'll": true, "she's": true, "should": true, "shouldn't": true,
	"so": true, "some": true, "such": true, "than": true, "that": true, "that's": true, "the": true,
	"their": true, "theirs": true, "them": true, "themselves": true, "then": true,
	"there": true, "there's": true, "these": true, "they": true, "they'd": true, "they'll": true,
	"they're": true, "they've": true, "this": true, "those": true, "through": true, "to": true, "too": true,
	"under": true, "until": true, "up": true, "very": true,
	"was": true, "wasn't": true, "we": true, "we'd": true, "we'll": true, "we're": true, "we've": true,
	"were": true, "weren't": true, "what": true, "what's": true, "when": true, "when's": true,
	"where": true, "where's": true, "which": true, "while": true, "who": true, "who's": true,
	"whom": true, "why": true, "why's": true, "with": true, "won't": true, "would": true, "wouldn't": true,
	"you": true, "you'd": true, "you'll": true, "you're": true, "you've": true, "your": true,
	"yours": true, "yourself": true, "yourselves": true,
}

// OmitCommonWords drops function words and recounts what remains.
func OmitCommonWords(index WordIndex) WordIndex {
	kept := make([]WordCount, 0, len(index.Words))
	total := 0
	for _, item := range index.Words {
		if commonWords[strings.ToLower(item.Word)] {
			continue
		}
		kept = append(kept, item)
		total += item.Count
	}
	index.Words = kept
	index.WordCount = total
	index.UniqueCount = len(kept)
	return index
}

// urlSkip are host labels that name the address itself, not the site.
var urlSkip = map[string]bool{
	"www": true, "www2": true, "m": true, "mobile": true, "amp": true,
	"feed": true, "feeds": true, "rss": true, "cdn": true, "static": true,
	"assets": true, "media": true, "img": true, "images": true,
	"com": true, "org": true, "net": true, "uk": true, "io": true, "co": true,
	"gov": true, "edu": true, "info": true, "html": true, "htm": true,
	"php": true, "asp": true, "aspx": true, "jp": true, "de": true, "fr": true,
	"au": true, "ca": true, "us": true, "nz": true, "eu": true,
}

// URLWords returns the site words in a URL host. www.theguardian.com yields
// theguardian and guardian. Path and query words stay, because those are the article.
func URLWords(raw string) map[string]bool {
	host := urlHost(raw)
	if host == "" {
		return nil
	}
	out := map[string]bool{}
	for _, label := range strings.Split(host, ".") {
		if urlSkip[label] {
			continue
		}
		for _, word := range tokenize(label) {
			if len(word) < 2 || urlSkip[word] {
				continue
			}
			out[word] = true
			if strings.HasPrefix(word, "the") && len(word) > 5 {
				rest := word[3:]
				if !urlSkip[rest] && hasLetterOrNumber(rest) {
					out[rest] = true
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func urlHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return ""
		}
	}
	return strings.ToLower(parsed.Hostname())
}

// OmitListedWords drops the named words and recounts what remains.
func OmitListedWords(index WordIndex, drop map[string]bool) WordIndex {
	if len(drop) == 0 {
		return index
	}
	kept := make([]WordCount, 0, len(index.Words))
	total := 0
	for _, item := range index.Words {
		if drop[strings.ToLower(item.Word)] {
			continue
		}
		kept = append(kept, item)
		total += item.Count
	}
	index.Words = kept
	index.WordCount = total
	index.UniqueCount = len(kept)
	return index
}
