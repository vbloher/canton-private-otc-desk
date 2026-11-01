package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func exercise(name string) json.RawMessage {
	return json.RawMessage(`{"name":"` + name + `"}`)
}

func Test_Combine_OneTransaction(t *testing.T) {
	rules := domain.Disclosed{TemplateID: "Rules", ContractID: "c-rules", CreatedEventBlob: "b-rules", SynchronizerID: "global"}
	round := domain.Disclosed{TemplateID: "Round", ContractID: "c-round", CreatedEventBlob: "b-round", SynchronizerID: "global"}
	factory := domain.Disclosed{TemplateID: "Factory", ContractID: "c-factory", CreatedEventBlob: "b-factory"}

	combined, ok := domain.Combine([]domain.Command{
		{Commands: []json.RawMessage{exercise("cc")}, Disclosed: []domain.Disclosed{rules, round}, SynchronizerID: "global"},
		{Commands: []json.RawMessage{exercise("cbtc")}, Disclosed: []domain.Disclosed{factory, rules}},
	})

	require.True(t, ok)
	require.Equal(t, domain.Command{
		Commands:       []json.RawMessage{exercise("cc"), exercise("cbtc")},
		Disclosed:      []domain.Disclosed{rules, round, factory},
		SynchronizerID: "global",
	}, combined)
}

func Test_Combine_RefusesSeveralSynchronizers(t *testing.T) {
	_, ok := domain.Combine([]domain.Command{
		{Commands: []json.RawMessage{exercise("cc")}, SynchronizerID: "global"},
		{Commands: []json.RawMessage{exercise("cbtc")}, SynchronizerID: "private"},
	})

	require.False(t, ok)
}
