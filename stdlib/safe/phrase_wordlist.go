package safe

import "strings"

// PhraseWordCount is the fixed size of the canonical wordlist used by Phrase.
// Power-of-two sizing yields a clean bits-per-word ratio (8 bits/word → byte-aligned entropy).
const PhraseWordCount = 256

// phraseWords is the canonical English wordlist for Phrase. Properties:
//   - exactly 256 unique words
//   - short (3–7 characters), common, casual-English familiar
//   - no homophones, no near-duplicates on the first three characters
//   - alphabetized for reviewability
//
// Any change to this list is a breaking change — existing phrases become unparseable.
const phraseWords = `
able acid acre agent album alert alien alloy amber anchor angel anger ankle apple arena armor
artist atlas atom auto avid awake axis baby bacon badge baker balm band bark barn base
bath beach beam bear bench berry bike bill bird black blade blaze bliss block blood bloom
blue blur board boat bold bolt bone book boot born bottle brain brass brave bread brick
bridge brook brown brush buddy budget build bulb bunny cabin cable cake calm camel camera candy
canyon cargo carrot castle cave cedar cello center chain chair cherry chest chime chip chorus church
cider circle cliff clock cloud coast cobra cocoa comet copper coral cosmic cotton couch cradle crane
crate crown crystal cube daisy dance dart dawn deep desk diamond disco doll dome donut dove
dozen drift drum dune dusk eagle earth echo edge eight ember emerald energy ether evil falcon
feast feather felt ferry field flag flame fleece flock flower flute focus forge frame frost fuel
galaxy gate giant glacier globe glow golden grape gravel green grove guard harbor harvest hatch hawk
heart helmet hero hickory honey hood horizon horn island ivory jade jelly kelp kitten ladder lantern
laser lemon linen lotus lunar magnet maple marble meadow melon mint moss needle nickel north ocean
olive opal orchid otter oyster paper parade peach pebble petal pine pixel planet poem puzzle quartz
quiet radio raven ribbon river robot rodeo rope rose rust saber sable safari salt satin scout
shadow silver smoke snow solar sonic spark spire spruce star steel stone storm sugar summit swan
`

// phraseLookup maps word → index for O(1) decode.
var phraseLookup = func() map[string]uint8 {
	list := phraseList()
	if len(list) != PhraseWordCount {
		panic("safe: phraseWords must contain exactly PhraseWordCount words; got " + itoa(len(list)))
	}
	lookup := make(map[string]uint8, PhraseWordCount)
	for i, w := range list {
		if _, exists := lookup[w]; exists {
			panic("safe: phraseWords contains duplicate: " + w)
		}
		lookup[w] = uint8(i)
	}
	return lookup
}()

func phraseList() []string {
	return strings.Fields(phraseWords)
}

// PhraseWordAt returns the canonical word at the given index (0..PhraseWordCount-1).
// Panics if idx is out of range.
func PhraseWordAt(idx int) string {
	list := phraseList()
	return list[idx]
}

// PhraseWordIndex returns the index of word in the canonical wordlist,
// or -1 if word is not present. Word matching is case-sensitive.
func PhraseWordIndex(word string) int {
	if idx, ok := phraseLookup[word]; ok {
		return int(idx)
	}
	return -1
}

// itoa is a tiny int → string helper to avoid importing strconv at init time.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
