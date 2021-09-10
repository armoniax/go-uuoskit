package main

// #include <stdint.h>
// static char* get_p(char **pp, int i)
// {
//	    return pp[i];
// }
import "C"

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"unsafe"

	"github.com/uuosio/go-secp256k1/secp256k1"
)

func renderData(data interface{}) *C.char {
	ret := map[string]interface{}{"data": data}
	result, _ := json.Marshal(ret)
	return C.CString(string(result))
}

func renderError(err error) *C.char {
	pc, fn, line, _ := runtime.Caller(1)
	errMsg := fmt.Sprintf("[error] in %s[%s:%d] %v", runtime.FuncForPC(pc).Name(), fn, line, err)
	ret := map[string]interface{}{"error": errMsg}
	result, _ := json.Marshal(ret)
	return C.CString(string(result))
}

//export init_
func init_() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	secp256k1.Init()
	if nil == GetABISerializer() {
		panic("abi serializer not initialized")
	}
}

//export say_hello_
func say_hello_(name *C.char) {
	_name := C.GoString(name)
	log.Println("hello,world", _name)
}

//export wallet_import_
func wallet_import_(name *C.char, priv *C.char) *C.char {
	_name := C.GoString(name)
	_priv := C.GoString(priv)
	err := GetWallet().Import(_name, _priv)
	if err != nil {
		return renderError(err)
	}
	log.Println("import", _name, _priv)
	return renderData("ok")
}

//export wallet_get_public_keys_
func wallet_get_public_keys_() *C.char {
	keys := GetWallet().GetPublicKeys()
	return renderData(keys)
}

//export wallet_sign_digest_
func wallet_sign_digest_(digest *C.char, pubKey *C.char) *C.char {
	_pubKey := C.GoString(pubKey)
	log.Println("++++++++wallet_sign_digest_:", C.GoString(digest))
	_digest, err := hex.DecodeString(C.GoString(digest))
	if err != nil {
		return renderError(err)
	}

	sign, err := GetWallet().Sign(_digest, _pubKey)
	if err != nil {
		return renderError(err)
	}
	return renderData(sign.String())
}

var gPackedTxs []*PackedTransaction

func validateIndex(idx C.int64_t) error {
	if idx < 0 || idx >= C.int64_t(len(gPackedTxs)) {
		return fmt.Errorf("invalid idx")
	}

	if gPackedTxs[idx] == nil {
		return fmt.Errorf("invalid idx")
	}

	return nil
}

//export transaction_new_
func transaction_new_(expiration C.int64_t, refBlock *C.char, chainId *C.char) C.int64_t {
	tx := NewTransaction(int(expiration))
	tx.SetReferenceBlock(C.GoString(refBlock))

	packedTx := NewPackedtransaction(tx)
	packedTx.SetChainId(C.GoString(chainId))
	if gPackedTxs == nil {
		//element at 0 not used
		gPackedTxs = make([]*PackedTransaction, 0, 10)
	}

	if len(gPackedTxs) >= 1024 {
		return C.int64_t(-1)
	}

	for i := 0; i < len(gPackedTxs); i++ {
		if gPackedTxs[i] == nil {
			gPackedTxs[i] = packedTx
			return C.int64_t(i)
		}
	}
	gPackedTxs = append(gPackedTxs, packedTx)
	return C.int64_t(len(gPackedTxs) - 1)
}

//export transaction_free_
func transaction_free_(_index C.int64_t) {
	index := int(_index)
	if index < 0 || index >= len(gPackedTxs) {
		return
	}
	gPackedTxs[int(index)] = nil
	return
}

//export transaction_add_action_
func transaction_add_action_(idx C.int64_t, account *C.char, name *C.char, data *C.char, permissions *C.char) *C.char {
	if err := validateIndex(idx); err != nil {
		return renderError(err)
	}

	_account := C.GoString(account)
	_name := C.GoString(name)
	_data := C.GoString(data)
	_permissions := C.GoString(permissions)

	var __data []byte
	__data, err := hex.DecodeString(_data)
	if err != nil {
		__data, err = GetABISerializer().PackActionArgs(_account, _name, []byte(_data))
		if err != nil {
			return renderError(err)
		}
	}

	perms := make(map[string]string)
	err = json.Unmarshal([]byte(_permissions), &perms)
	if err != nil {
		return renderError(err)
	}

	action := NewAction(NewName(_account), NewName(_name))
	action.SetData(__data)
	for k, v := range perms {
		action.AddPermission(NewName(k), NewName(v))
	}
	gPackedTxs[idx].tx.AddAction(action)

	//reset PackedTx
	gPackedTxs[idx].PackedTx = nil
	return renderData("ok")
}

//export transaction_sign_
func transaction_sign_(idx C.int64_t, pub *C.char) *C.char {
	if err := validateIndex(idx); err != nil {
		return renderError(err)
	}

	_pub := C.GoString(pub)
	err := gPackedTxs[idx].Sign(_pub)
	if err != nil {
		return renderError(err)
	}
	return renderData("ok")
}

//export transaction_pack_
func transaction_pack_(idx C.int64_t) *C.char {
	if err := validateIndex(idx); err != nil {
		return renderError(err)
	}

	result := gPackedTxs[idx].String()
	return renderData(result)
}

//export abiserializer_set_contract_abi_
func abiserializer_set_contract_abi_(account *C.char, abi *C.char, length C.int) *C.char {
	_account := C.GoString(account)
	_abi := C.GoBytes(unsafe.Pointer(abi), length)
	err := GetABISerializer().AddContractABI(_account, _abi)
	if err != nil {
		return renderError(err)
	}
	return renderData("ok")
}

//export abiserializer_pack_action_args_
func abiserializer_pack_action_args_(contractName *C.char, actionName *C.char, args *C.char, args_len C.int) *C.char {
	_contractName := C.GoString(contractName)
	_actionName := C.GoString(actionName)
	_args := C.GoBytes(unsafe.Pointer(args), args_len)
	result, err := GetABISerializer().PackActionArgs(_contractName, _actionName, _args)
	if err != nil {
		return renderError(err)
	}
	return renderData(hex.EncodeToString(result))
}

//export abiserializer_unpack_action_args_
func abiserializer_unpack_action_args_(contractName *C.char, actionName *C.char, args *C.char) *C.char {
	_contractName := C.GoString(contractName)
	_actionName := C.GoString(actionName)
	_args := C.GoString(args)
	__args, err := hex.DecodeString(_args)
	if err != nil {
		return renderError(err)
	}
	result, err := GetABISerializer().UnpackActionArgs(_contractName, _actionName, __args)
	if err != nil {
		return renderError(err)
	}
	return renderData(string(result))
}

//export abiserializer_pack_abi_type_
func abiserializer_pack_abi_type_(contractName *C.char, actionName *C.char, args *C.char, args_len C.int) *C.char {
	_contractName := C.GoString(contractName)
	_actionName := C.GoString(actionName)
	_args := C.GoBytes(unsafe.Pointer(args), args_len)
	result, err := GetABISerializer().PackAbiType(_contractName, _actionName, _args)
	if err != nil {
		return renderError(err)
	}
	return renderData(hex.EncodeToString(result))
}

//export abiserializer_unpack_abi_type_
func abiserializer_unpack_abi_type_(contractName *C.char, actionName *C.char, args *C.char) *C.char {
	_contractName := C.GoString(contractName)
	_actionName := C.GoString(actionName)
	_args := C.GoString(args)
	__args, err := hex.DecodeString(_args)
	if err != nil {
		return renderError(err)
	}
	result, err := GetABISerializer().UnpackAbiType(_contractName, _actionName, __args)
	if err != nil {
		return renderError(err)
	}
	return renderData(string(result))
}

//export abiserializer_is_abi_cached_
func abiserializer_is_abi_cached_(contractName *C.char) C.int {
	_contractName := C.GoString(contractName)
	result := GetABISerializer().IsAbiCached(_contractName)
	if result {
		return 1
	} else {
		return 0
	}
}

//export s2n_
func s2n_(s *C.char) C.uint64_t {
	return C.uint64_t(S2N(C.GoString(s)))
}

//export n2s_
func n2s_(n C.uint64_t) *C.char {
	return C.CString(N2S(uint64(n)))
}

//symbol to uint64
//export sym2n_
func sym2n_(str_symbol *C.char, precision C.uint64_t) C.uint64_t {
	v := NewSymbol(C.GoString(str_symbol), int(uint64(precision))).Value
	return C.uint64_t(v)
}

//export abiserializer_pack_abi_
func abiserializer_pack_abi_(str_abi *C.char) *C.char {
	_str_abi := C.GoString(str_abi)
	result, err := GetABISerializer().PackABI(_str_abi)
	if err != nil {
		return renderError(err)
	}
	return renderData(hex.EncodeToString(result))
}

//export abiserializer_unpack_abi_
func abiserializer_unpack_abi_(abi *C.char, length C.int) *C.char {
	_abi := C.GoBytes(unsafe.Pointer(abi), length)
	result, err := GetABISerializer().UnpackABI(_abi)
	if err != nil {
		return renderError(err)
	}
	return renderData(result)
}

//export crypto_sign_digest_
func crypto_sign_digest_(digest *C.char, privateKey *C.char) *C.char {
	log.Println(C.GoString(digest), C.GoString(privateKey))

	_privateKey, err := secp256k1.NewPrivateKeyFromBase58(C.GoString(privateKey))
	if err != nil {
		return renderError(err)
	}

	_digest, err := hex.DecodeString(C.GoString(digest))
	if err != nil {
		return renderError(err)
	}

	result, err := _privateKey.Sign(_digest)
	if err != nil {
		return renderError(err)
	}
	return renderData(result.String())
}

//export crypto_get_public_key_
func crypto_get_public_key_(privateKey *C.char) *C.char {
	_privateKey, err := secp256k1.NewPrivateKeyFromBase58(C.GoString(privateKey))
	if err != nil {
		return renderError(err)
	}

	pub := _privateKey.GetPublicKey()
	return renderData(pub.String())
}

//export crypto_recover_key_
func crypto_recover_key_(digest *C.char, signature *C.char) *C.char {
	_digest, err := hex.DecodeString(C.GoString(digest))
	if err != nil {
		return renderError(err)
	}

	_signature, err := secp256k1.NewSignatureFromBase58(C.GoString(signature))
	if err != nil {
		return renderError(err)
	}

	pub, err := secp256k1.Recover(_digest, _signature)
	if err != nil {
		return renderError(err)
	}
	return renderData(pub.String())
}
