# Paladin blockchain transaction manager

TBC: the current thinking is the [FireFly Transaction Manager](https://github.com/hyperledger/firefly-transaction-manager) should be reused for blockchain transaction submission and management. Work needs to be done to expose top-level firefly-transaction-manager APIs as a Golang interface.

After that work is done, this module may be deleted or contains a customized tx handler that is optimized for Paladin blockchain transaction submission pattern