# Siege

This testing tool surrounds [go-ethereum](https://github.com/ethereum/go-ethereum) with [cannon](https://github.com/ethereum-optimism/cannon) 
to catch the blocks of [retesteth](https://github.com/ethereum/retesteth) going into go-ethereum and test cannon with them.

## Usage

1. Start Go-ethereum, with `test`, `debug` and `eth` RPC namespaces enabled.
2. Start Siege, pointed to the Go-ethereum RPC and to the cannon executable.
3. Run retesteth, configured to point at the RPC endpoint of Siege
4. Every RPC call will be forwarded to the node.
5. Every `test_importRawBlock` RPC call will be parsed, forwarded to the node, and after the node returns run cannon before finally returning a response to retesteth.


## License

MIT License, see [`LICENSE`](./LICENSE) file.
