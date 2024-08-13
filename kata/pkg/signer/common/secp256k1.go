package common

import "github.com/hyperledger/firefly-signer/pkg/secp256k1"

// We use the ethereum convention of R,S,V for compact packing (mentioned because Golang tends to prefer V,R,S)
func CompactRSV(sig *secp256k1.SignatureData) []byte {
	signatureBytes := make([]byte, 65)
	sig.R.FillBytes(signatureBytes[0:32])
	sig.S.FillBytes(signatureBytes[32:64])
	signatureBytes[64] = byte(sig.V.Int64())
	return signatureBytes
}
