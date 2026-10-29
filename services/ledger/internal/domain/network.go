package domain

type Network string

const (
	NetworkDevNet  Network = "devnet"
	NetworkTestNet Network = "testnet"
	NetworkMainNet Network = "mainnet"
)

func (n *Network) UnmarshalText(text []byte) error {
	switch network := Network(text); network {
	case NetworkDevNet, NetworkTestNet, NetworkMainNet:
		*n = network

		return nil
	default:
		return Invalidf("network %q: want devnet, testnet or mainnet", text)
	}
}
