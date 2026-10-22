package domain

import "strings"

// HouseMaker is one entry of HOUSE_MAKERS: a hosted party and the API key its bot signs in with.
type HouseMaker struct {
	Hint   string
	APIKey string
}

// HouseMakers is the HOUSE_MAKERS setting, "hint:key,hint:key".
type HouseMakers []HouseMaker

func (h *HouseMakers) UnmarshalText(text []byte) error {
	list := strings.TrimSpace(string(text))
	if list == "" {
		*h = nil

		return nil
	}

	var (
		makers = HouseMakers{}
		hints  = map[string]struct{}{}
		keys   = map[string]struct{}{}
	)

	// Entries are never echoed back: they contain API keys.
	for i, entry := range strings.Split(list, ",") {
		hint, key, ok := strings.Cut(strings.TrimSpace(entry), ":")
		hint, key = strings.TrimSpace(hint), strings.TrimSpace(key)

		if !ok || hint == "" || key == "" {
			return Invalidf("house maker %d must look like hint:key", i+1)
		}

		_, hintSeen := hints[hint]
		_, keySeen := keys[key]

		if hintSeen || keySeen {
			return Invalidf("house maker %d repeats a hint or a key", i+1)
		}

		hints[hint], keys[key] = struct{}{}, struct{}{}
		makers = append(makers, HouseMaker{Hint: hint, APIKey: key})
	}

	*h = makers

	return nil
}
