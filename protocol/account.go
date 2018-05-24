package protocol

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/big"
)

type Account struct {
	Address            [64]byte  // 64 Byte
	Issuer             [32]byte  // 32 Byte
	Balance            uint64    // 8 Byte
	TxCnt              uint32    // 4 Byte
	IsStaking          bool      // 1 Byte
	HashedSeed         [32]byte  // 32 Byte
	StakingBlockHeight uint32    // 4 Byte
	Contract           []byte    // Arbitrary length
	ContractVariables  []big.Int // Arbitrary length
}

func NewAccount(address [64]byte, issuer [32]byte, balance uint64, isStaking bool, hashedSeed [32]byte, contract []byte, contractVariables []big.Int) Account {
	newAcc := Account{
		address,
		issuer,
		balance,
		0,
		isStaking,
		hashedSeed,
		0,
		contract,
		contractVariables,
	}
	return newAcc
}

func (acc *Account) Hash() (hash [32]byte) {
	if acc == nil {
		return [32]byte{}
	}
	return SerializeHashContent(acc.Address)
}

func (acc *Account) Encode() (encodedAcc []byte) {
	if acc == nil {
		return nil
	}

	encodeData := Account{
		Address:            acc.Address,
		Issuer:             acc.Issuer,
		Balance:            acc.Balance,
		TxCnt:              acc.TxCnt,
		IsStaking:          acc.IsStaking,
		HashedSeed:         acc.HashedSeed,
		StakingBlockHeight: acc.StakingBlockHeight,
		Contract:           acc.Contract,
		ContractVariables:  acc.ContractVariables,
	}

	buffer := new(bytes.Buffer)
	gob.NewEncoder(buffer).Encode(encodeData)
	return buffer.Bytes()
}

func (*Account) Decode(encodedAcc []byte) (acc *Account) {
	var decoded Account
	buffer := bytes.NewBuffer(encodedAcc)
	decoder := gob.NewDecoder(buffer)
	decoder.Decode(&decoded)
	return &decoded
}

func (acc Account) String() string {
	addressHash := SerializeHashContent(acc.Address)
	return fmt.Sprintf("Hash: %x, Address: %x, Issuer: %x, TxCnt: %v, Balance: %v, IsStaking: %v, HashedSeed: %x, StakingBlockHeight: %v, Contract: %v, ContractVariables: %v", addressHash[0:8], acc.Address[0:8], acc.Issuer[0:8], acc.TxCnt, acc.Balance, acc.IsStaking, acc.HashedSeed[0:8], acc.StakingBlockHeight, acc.Contract, acc.ContractVariables)
}
