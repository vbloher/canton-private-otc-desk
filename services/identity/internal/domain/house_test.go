package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_HouseMakers_UnmarshalText(t *testing.T) {
	var makers domain.HouseMakers

	require.NoError(t, makers.UnmarshalText([]byte(" makerA:demo-key-a , makerB:demo-key-b ")))
	require.Equal(t, domain.HouseMakers{
		{Hint: "makerA", APIKey: "demo-key-a"},
		{Hint: "makerB", APIKey: "demo-key-b"},
	}, makers)
}

func Test_HouseMakers_UnmarshalText_Empty(t *testing.T) {
	makers := domain.HouseMakers{{Hint: "old", APIKey: "old"}}

	require.NoError(t, makers.UnmarshalText([]byte("")))
	require.Empty(t, makers, "an empty list means no house makers")
}

func Test_HouseMakers_UnmarshalText_Rejects(t *testing.T) {
	for name, text := range map[string]string{
		"no key":         "makerA",
		"empty key":      "makerA:",
		"empty hint":     ":demo-key-a",
		"empty entry":    "makerA:demo-key-a,,makerB:demo-key-b",
		"duplicate hint": "makerA:key-1,makerA:key-2",
		"duplicate key":  "makerA:key-1,makerB:key-1",
	} {
		var makers domain.HouseMakers

		err := makers.UnmarshalText([]byte(text))
		require.ErrorIs(t, err, domain.ErrValidation, name)
	}
}

func Test_HouseMakers_KeyMayContainColons(t *testing.T) {
	var makers domain.HouseMakers

	require.NoError(t, makers.UnmarshalText([]byte("makerA:k:e:y")))
	require.Equal(t, domain.HouseMakers{{Hint: "makerA", APIKey: "k:e:y"}}, makers)
}
