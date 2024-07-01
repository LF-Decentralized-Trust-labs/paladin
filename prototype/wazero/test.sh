#!/bin/bash

curl -X POST "http://localhost:1337/" -H "Content-Type: application/json" -d @- <<EOF
{
  "method": "TokenA.Invoke",
  "params": [
    {
      "function": "Transfer",
      "input": {
        "foo": 40,
        "bar": 2
      }
    }
  ],
  "jsonrpc": "2.0",
  "id": 1
}
EOF

curl -X POST "http://localhost:1337/" -H "Content-Type: application/json" -d @- <<EOF
{
  "method": "TokenB.Invoke",
  "params": [
    {
      "function": "Transfer",
      "input": {
        "foo": 40,
        "bar": 2
      }
    }
  ],
  "jsonrpc": "2.0",
  "id": 1
}
EOF

