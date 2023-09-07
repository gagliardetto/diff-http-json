# diff-http-json

Sends the same HTTP request to two different URLs and compares the JSON responses.

## Usage

```
diff-http-json [options] <body>
```

# diff getTransaction

```
diff-http-json \
	--rpc=http://localhost:8899 \
	--rpc=https://api.mainnet-beta.solana.com \
	'{
	"jsonrpc": "2.0",
	"id": "99",
	"method": "getTransaction",
	"params": [
		"3ZoKehx5haKmM1r74Ni9Ezxc6Sa2vLqRQgKisETGUjLK3KX45yRTAbN4xZ4LXt9jXBBozvjQ4qTz5eJtq3PD6j2P",
		{
			"encoding": "json",
			"maxSupportedTransactionVersion": 0
		}
	]
}'

```

# diff getBlock

```
diff-http-json \
	--rpc=http://localhost:8899 \
	--rpc=https://api.mainnet-beta.solana.com \
	'{
	"jsonrpc": "2.0",
	"id": "99",
	"method": "getBlock",
	"params": [
		211247999,
		{
			"encoding": "base64",
			"maxSupportedTransactionVersion": 0
		}
	]
}'

```
