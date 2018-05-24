package vm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"

	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

type Context interface {
	GetContract() []byte
	GetContractVariable(index int) (big.Int, error)
	SetContractVariable(index int, value big.Int) error
	GetAddress() [64]byte
	GetIssuer() [32]byte
	GetBalance() uint64
	GetSender() [32]byte
	GetAmount() uint64
	GetTransactionData() []byte
	GetFee() uint64
	GetSig1() [64]byte
}

type VM struct {
	code            []byte
	pc              int // Program counter
	evaluationStack *Stack
	callStack       *CallStack
	context         Context
}

func NewVM(context Context) VM {
	return VM{
		code:            []byte{},
		pc:              0,
		evaluationStack: NewStack(),
		callStack:       NewCallStack(),
		context:         context,
	}
}

func NewTestVM(byteCode []byte) VM {
	return VM{
		code:            []byte{},
		pc:              0,
		evaluationStack: NewStack(),
		callStack:       NewCallStack(),
		context:         NewMockContext(byteCode),
	}
}

// Private function, that can be activated by Exec call, useful for debugging
func (vm *VM) trace() {
	stack := vm.evaluationStack
	addr := vm.pc
	opCode := OpCodes[int(vm.code[vm.pc])]
	var args []byte

	switch opCode.Name {
	case "push":
		nargs := int(vm.code[vm.pc+1])

		if vm.pc+nargs < (len(vm.code) - vm.pc) {
			args = vm.code[vm.pc+2 : vm.pc+nargs+3]
			fmt.Printf("%04d: %-6s %-10v %v\n", addr, opCode.Name, ByteArrayToInt(args), stack)
		}

		//TODO - Fix CALLEXT case, leads to index out of bounds exception
	/*case "callext":
	address := vm.code[vm.pc+1 : vm.pc+33]
	functionHash := vm.code[vm.pc+33 : vm.pc+37]
	nargs := int(vm.code[vm.pc+37])

	fmt.Printf("%04d: %-6s %x %x %v %v\n", addr, opCode.Name, address, functionHash, nargs, stack)
	*/

	case "mappush":
	case "mapgetval":
	case "newarr":
	case "arrappend":
	case "arrinsert":
	case "arrremove":
	case "arrat":
		args = vm.code[vm.pc+1 : vm.pc+opCode.Nargs+1]
		fmt.Printf("%04d: %-6s %v ", addr, opCode.Name, args)

		for _, e := range stack.Stack {
			fmt.Printf("%# x", e.Bytes())
			fmt.Printf("\n")
		}

		fmt.Printf("\n")

	default:
		args = vm.code[vm.pc+1 : vm.pc+opCode.Nargs+1]
		fmt.Printf("%04d: %-6s %v %v\n", addr, opCode.Name, args, stack)
	}
}

func (vm *VM) Exec(trace bool) bool {

	vm.code = vm.context.GetContract()

	if len(vm.code) > 100000 {
		vm.evaluationStack.Push(StrToBigInt("Instruction set to big"))
		return false
	}

	fee := vm.context.GetFee()

	// Infinite Loop until return called
	for {
		if trace {
			vm.trace()
		}

		// Fetch
		opCode, err := vm.fetch()

		if err != nil {
			vm.evaluationStack.Push(StrToBigInt(err.Error()))
			return false
		}

		// Return false if instruction is not an opCode
		if len(OpCodes) < int(opCode) {
			vm.evaluationStack.Push(StrToBigInt("Not a valid opCode"))
			return false
		}

		// Subtract gas used for operation
		if fee < OpCodes[int(opCode)].gasPrice {
			vm.evaluationStack.Push(StrToBigInt("out of gas"))
			return false
		} else {
			fee -= OpCodes[int(opCode)].gasPrice
		}

		// Decode
		switch opCode {

		case PUSH:
			arg, errArg1 := vm.fetch()
			byteCount := int(arg) + 1 // Amount of bytes pushed, maximum amount of bytes that can be pushed is 256
			bytes, errArg2 := vm.fetchMany(byteCount)

			if !vm.checkErrors(errArg1, errArg2) {
				return false
			}

			var bigInt big.Int
			bigInt.SetBytes(bytes)

			err = vm.evaluationStack.Push(bigInt)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case DUP:
			val, err := vm.evaluationStack.Peek()

			if !vm.checkErrors(err) {
				return false
			}

			err = vm.evaluationStack.Push(val)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ROLL:
			arg, err := vm.fetch() // arg shows how many have to be rolled
			index := vm.evaluationStack.GetLength() - (int(arg) + 2)

			if !vm.checkErrors(err) {
				return false
			}

			if index != -1 {
				if int(arg) >= vm.evaluationStack.GetLength() {
					vm.evaluationStack.Push(StrToBigInt("index out of bounds"))
					return false
				}

				newTos, err := vm.evaluationStack.PopIndexAt(index)

				if err != nil {
					vm.evaluationStack.Push(StrToBigInt(err.Error()))
					return false
				}

				err = vm.evaluationStack.Push(newTos)

				if err != nil {
					vm.evaluationStack.Push(StrToBigInt(err.Error()))
					return false
				}
			}

		case POP:
			_, rerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr) {
				return false
			}

		case ADD:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			left.Add(&left, &right)
			err := vm.evaluationStack.Push(left)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case SUB:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			left.Sub(&left, &right)
			err := vm.evaluationStack.Push(left)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MULT:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			left.Mul(&left, &right)
			err := vm.evaluationStack.Push(left)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case DIV:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if right.Cmp(big.NewInt(0)) == 0 {
				vm.evaluationStack.Push(StrToBigInt("Division by Zero"))
				return false
			}

			left.Div(&left, &right)
			err := vm.evaluationStack.Push(left)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MOD:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if right.Cmp(big.NewInt(0)) == 0 {
				vm.evaluationStack.Push(StrToBigInt("Division by Zero"))
				return false
			}

			left.Mod(&left, &right)
			err := vm.evaluationStack.Push(left)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case NEG:
			tos, err := vm.evaluationStack.Pop()

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			tos.Neg(&tos)

			vm.evaluationStack.Push(tos)

		case EQ:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) == 0 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case NEQ:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) != 0 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case LT:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) == -1 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case GT:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) == 1 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case LTE:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) == -1 || left.Cmp(&right) == 0 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case GTE:
			right, rerr := vm.evaluationStack.Pop()
			left, lerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(rerr, lerr) {
				return false
			}

			if left.Cmp(&right) == 1 || left.Cmp(&right) == 0 {
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case SHIFTL:
			nrOfShifts, errArg := vm.fetch()
			tos, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArg, errStack) {
				return false
			}

			tos.Lsh(&tos, uint(nrOfShifts))
			err = vm.evaluationStack.Push(tos)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case SHIFTR:
			nrOfShifts, errArg := vm.fetch()
			tos, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArg, errStack) {
				return false
			}

			tos.Rsh(&tos, uint(nrOfShifts))
			err = vm.evaluationStack.Push(tos)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case NOP:
			_, err := vm.fetch()

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case JMP:
			nextInstruction, err := vm.fetchMany(2)

			if !vm.checkErrors(err) {
				return false
			}

			var jumpTo big.Int
			jumpTo.SetBytes(nextInstruction)

			vm.pc = int(jumpTo.Int64())

		case JMPIF:
			nextInstruction, errArg := vm.fetchMany(2)
			right, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArg, errStack) {
				return false
			}

			var jumpTo big.Int
			jumpTo.SetBytes(nextInstruction)

			if right.Int64() == 1 {
				vm.pc = int(jumpTo.Int64())
			}

		case CALL:
			returnAddressBytes, errArg1 := vm.fetchMany(2) // Shows where to jump after executing
			argsToLoad, errArg2 := vm.fetch()              // Shows how many elements have to be popped from evaluationStack

			if !vm.checkErrors(errArg1, errArg2) {
				return false
			}

			var returnAddress big.Int
			returnAddress.SetBytes(returnAddressBytes)

			if int(returnAddress.Int64()) == 0 || int(returnAddress.Int64()) > len(vm.code) {
				vm.evaluationStack.Push(StrToBigInt("ReturnAddress out of bounds"))
				return false
			}

			frame := &Frame{returnAddress: vm.pc, variables: make(map[int]big.Int)}

			for i := int(argsToLoad) - 1; i >= 0; i-- {
				frame.variables[i], err = vm.evaluationStack.Pop()
				if err != nil {
					vm.evaluationStack.Push(StrToBigInt(err.Error()))
					return false
				}
			}

			vm.callStack.Push(frame)
			vm.pc = int(returnAddress.Int64())

		case CALLIF:
			returnAddressBytes, errArg1 := vm.fetchMany(2) // Shows where to jump after executing
			argsToLoad, errArg2 := vm.fetch()              // Shows how many elements have to be popped from evaluationStack
			right, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArg1, errArg2, errStack) {
				return false
			}

			if right.Int64() == 1 {
				var returnAddress big.Int
				returnAddress.SetBytes(returnAddressBytes)

				if int(returnAddress.Int64()) == 0 || int(returnAddress.Int64()) > len(vm.code) {
					vm.evaluationStack.Push(StrToBigInt("ReturnAddress out of bounds"))
					return false
				}

				frame := &Frame{returnAddress: vm.pc, variables: make(map[int]big.Int)}

				for i := int(argsToLoad) - 1; i >= 0; i-- {
					frame.variables[i], err = vm.evaluationStack.Pop()
					if err != nil {
						vm.evaluationStack.Push(StrToBigInt(err.Error()))
						return false
					}
				}
				vm.callStack.Push(frame)
				vm.pc = int(returnAddress.Int64())
			}

		case CALLEXT:
			transactionAddress, errArg1 := vm.fetchMany(32) // Addresses are 32 bytes (var name: transactionAddress)
			functionHash, errArg2 := vm.fetchMany(4)        // Function hash identifies function in external smart contract, first 4 byte of SHA3 hash (var name: functionHash)
			argsToLoad, errArg3 := vm.fetch()               // Shows how many arguments to pop from stack and pass to external function (var name: argsToLoad)

			if !vm.checkErrors(errArg1, errArg2, errArg3) {
				return false
			}

			fmt.Sprint("CALLEXT", transactionAddress, functionHash, argsToLoad)
			//TODO: Invoke new transaction with function hash and arguments, waiting for integration in bazo blockchain to finish

		case RET:
			callstackTos, err := vm.callStack.Peek()

			if !vm.checkErrors(err) {
				return false
			}

			vm.callStack.Pop()
			vm.pc = callstackTos.returnAddress

		case SIZE:
			right, err := vm.evaluationStack.Pop()

			if !vm.checkErrors(err) {
				return false
			}

			err = vm.evaluationStack.Push(*big.NewInt(int64(getElementMemoryUsage(right.BitLen()))))

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case SSTORE:
			index, errArgs := vm.fetch()
			value, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArgs, errStack) {
				return false
			}

			err = vm.context.SetContractVariable(int(index), value)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case STORE:
			address, errArgs := vm.fetch()
			right, errStack := vm.evaluationStack.Pop()

			if !vm.checkErrors(errArgs, errStack) {
				return false
			}

			callstackTos, err := vm.callStack.Peek()

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			callstackTos.variables[int(address)] = right

		case SLOAD:
			index, err := vm.fetch()

			if !vm.checkErrors(err) {
				return false
			}

			value, err := vm.context.GetContractVariable(int(index))

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			err = vm.evaluationStack.Push(value)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case LOAD:
			address, errArg := vm.fetch()
			callstackTos, errCallStack := vm.callStack.Peek()

			if !vm.checkErrors(errArg, errCallStack) {
				return false
			}

			val := callstackTos.variables[int(address)]

			err := vm.evaluationStack.Push(val)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ADDRESS:
			address := new(big.Int)
			a := vm.context.GetAddress()
			address.SetBytes(a[:])
			err := vm.evaluationStack.Push(*address)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ISSUER:
			issuer := new(big.Int)
			i := vm.context.GetIssuer()
			issuer.SetBytes(i[:])
			err := vm.evaluationStack.Push(*issuer)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case BALANCE:
			balance := new(big.Int)
			ba := make([]byte, 8)
			binary.LittleEndian.PutUint64(ba, vm.context.GetBalance())
			balance.SetBytes(ba)

			err := vm.evaluationStack.Push(*balance)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case CALLER:
			caller := new(big.Int)
			c := vm.context.GetSender()
			caller.SetBytes(c[:])
			err := vm.evaluationStack.Push(*caller)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case CALLVAL:
			value := new(big.Int)

			ba := make([]byte, 8)
			binary.LittleEndian.PutUint64(ba, vm.context.GetAmount())
			value.SetBytes(ba)

			err := vm.evaluationStack.Push(*value)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case CALLDATA:
			td := vm.context.GetTransactionData()
			for i := 0; i < len(td); i++ {
				length := int(td[i]) // Length of parameters

				if len(td)-i-1 <= length {
					vm.evaluationStack.Push(StrToBigInt("Index out of bounds"))
					return false
				}

				err := vm.evaluationStack.Push(*big.NewInt(0).SetBytes(td[i+1 : i+length+2]))

				if err != nil {
					vm.evaluationStack.Push(StrToBigInt(err.Error()))
					return false
				}

				i += int(td[i]) + 1 // Increase to next parameter length
			}

		case NEWMAP:
			m := NewMap()
			err = vm.evaluationStack.Push(m.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MAPPUSH:
			k, kerr := vm.evaluationStack.Pop()
			v, verr := vm.evaluationStack.Pop()
			mbi, mbierr := vm.evaluationStack.Pop()
			m, merr := MapFromBigInt(mbi)

			if !vm.checkErrors(kerr, verr, mbierr, merr) {
				return false
			}

			m.Append(k.Bytes(), v.Bytes())
			err := vm.evaluationStack.Push(m.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MAPGETVAL:
			kbi, kerr := vm.evaluationStack.Pop()
			mbi, mbierr := vm.evaluationStack.Pop()
			k := kbi.Bytes()
			m, merr := MapFromBigInt(mbi)
			v, err := m.GetVal(k)

			if !vm.checkErrors(kerr, mbierr, merr, err) {
				return false
			}

			result := big.Int{}
			result.SetBytes(v)
			err = vm.evaluationStack.Push(result)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MAPSETVAL:
			k, kerr := vm.evaluationStack.Pop()
			v, verr := vm.evaluationStack.Pop()
			mbi, mbierr := vm.evaluationStack.Pop()
			m, merr := MapFromBigInt(mbi)

			if !vm.checkErrors(kerr, verr, mbierr, merr) {
				return false
			}

			err := m.SetVal(k.Bytes(), v.Bytes())
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			err = vm.evaluationStack.Push(m.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case MAPREMOVE:
			kbi, kbierr := vm.evaluationStack.Pop()
			mbi, mbierr := vm.evaluationStack.Pop()
			m, merr := MapFromBigInt(mbi)

			if !vm.checkErrors(kbierr, mbierr, merr) {
				return false
			}

			m.Remove(kbi.Bytes())
			err = vm.evaluationStack.Push(m.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case NEWARR:
			a := NewArray()
			vm.evaluationStack.Push(a.ToBigInt())

		case ARRAPPEND:
			v, verr := vm.evaluationStack.Pop()
			a, aerr := vm.evaluationStack.Pop()

			if !vm.checkErrors(verr, aerr) {
				return false
			}

			arr, err := ArrayFromBigInt(a)
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			err = arr.Append(v)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt("Invalid argument size of ARRAPPEND"))
				return false
			}

			err = vm.evaluationStack.Push(arr.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ARRINSERT:
			i, err := vm.evaluationStack.Pop()
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			if len(i.Bytes()) > 2 {
				vm.evaluationStack.Push(StrToBigInt("Wrong index size"))
				return false
			}

			e, err := vm.evaluationStack.Pop()
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			a, err := vm.evaluationStack.Pop()
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			arr, err := ArrayFromBigInt(a)
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			index, err := ByteArrayToUI16(i.Bytes())
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			size, err := arr.getSize()
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			if index >= size {
				vm.evaluationStack.Push(StrToBigInt("Index out of bounds"))
				return false
			}

			err = arr.Insert(index, e)
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			err = vm.evaluationStack.Push(arr.ToBigInt())
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ARRREMOVE:
			a, aerr := vm.evaluationStack.Pop()
			ba, errArgs := vm.fetchMany(2)
			index, err := ByteArrayToUI16(ba)

			if !vm.checkErrors(aerr, errArgs) {
				return false
			}

			arr, perr := ArrayFromBigInt(a)
			if perr != nil {
				vm.evaluationStack.Push(StrToBigInt(perr.Error()))
				return false
			}

			rerr := arr.Remove(index)
			if rerr != nil {
				vm.evaluationStack.Push(StrToBigInt(rerr.Error()))
				return false
			}

			err = vm.evaluationStack.Push(arr.ToBigInt())

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case ARRAT:
			a, err := vm.evaluationStack.Peek()
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			ba, err := vm.fetchMany(2)
			index, conversionErr := ByteArrayToUI16(ba)

			if conversionErr != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			arr, err := ArrayFromBigInt(a)
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			e, err := arr.At(index)
			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}
			result := big.Int{}
			result.SetBytes(e)

			err = vm.evaluationStack.Push(result)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case SHA3:
			right, err := vm.evaluationStack.Pop()

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

			hasher := sha3.New256()
			hasher.Write(right.Bytes())
			hash := hasher.Sum(nil)

			var bigInt big.Int
			bigInt.SetBytes(hash)

			err = vm.evaluationStack.Push(bigInt)

			if err != nil {
				vm.evaluationStack.Push(StrToBigInt(err.Error()))
				return false
			}

		case CHECKSIG:
			publicKeySig, errArg1 := vm.evaluationStack.Pop() // PubKeySig
			hash, errArg2 := vm.evaluationStack.Pop()         // Hash

			if !vm.checkErrors(errArg1, errArg2) {
				return false
			}

			if len(publicKeySig.Bytes()) != 64 {
				vm.evaluationStack.Push(StrToBigInt("Not a valid address"))
				return false
			}

			if len(hash.Bytes()) != 32 {
				vm.evaluationStack.Push(StrToBigInt("Not a valid hash"))
				return false
			}

			pubKey1Sig1, pubKey2Sig1 := new(big.Int), new(big.Int)
			r, s := new(big.Int), new(big.Int)

			pubKey1Sig1.SetBytes(publicKeySig.Bytes()[:32])
			pubKey2Sig1.SetBytes(publicKeySig.Bytes()[32:])

			sig1 := vm.context.GetSig1()
			r.SetBytes(sig1[:32])
			s.SetBytes(sig1[32:])

			pubKey := ecdsa.PublicKey{elliptic.P256(), pubKey1Sig1, pubKey2Sig1}

			if ecdsa.Verify(&pubKey, hash.Bytes(), r, s) {
				fmt.Println("Valid Sig", pubKey, hash.Bytes())
				vm.evaluationStack.Push(*big.NewInt(1))
			} else {
				vm.evaluationStack.Push(*big.NewInt(0))
			}

		case ERRHALT:
			return false

		case HALT:
			return true
		}
	}
}

func (vm *VM) fetch() (element byte, err error) {
	tempPc := vm.pc
	if len(vm.code) > tempPc {
		vm.pc++
		return vm.code[tempPc], nil
	} else {
		return 0, errors.New("instructionSet out of bounds")
	}
}

func (vm *VM) fetchMany(argument int) (elements []byte, err error) {
	tempPc := vm.pc
	if len(vm.code)-tempPc > argument {
		vm.pc += argument
		return vm.code[tempPc : tempPc+argument], nil
	} else {
		return []byte{}, errors.New("instructionSet out of bounds")
	}
}

func (vm *VM) checkErrors(errors ...error) bool {
	for i, err := range errors {
		if err != nil {
			vm.evaluationStack.Push(StrToBigInt(errors[i].Error()))
			return false
		}
	}
	return true
}

func (vm *VM) GetErrorMsg() string {
	tos, err := vm.evaluationStack.Peek()
	if err != nil {
		return "Peek on empty Stack"
	}
	return BigIntToString(tos)
}
