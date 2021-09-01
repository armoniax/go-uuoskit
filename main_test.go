package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"fmt"
	"testing"

	"github.com/iancoleman/orderedmap"
	"github.com/learnforpractice/go-secp256k1/secp256k1"
)

func TestOrderedMap(t *testing.T) {
	t.Log("hello,world")
	o := orderedmap.New()
	json.Unmarshal([]byte(`{"hello": 123, "a": "b", "c": {"hello":123, "a": "b"}}`), &o)
	fmt.Println(o)
	s, _ := json.Marshal(o)
	fmt.Println(string(s))
	for _, k := range o.Keys() {
		v, _ := o.Get(k)
		fmt.Printf("+++++++k %v, v %v, %T\n", k, v, v)
	}
}

func TestCrypto(t *testing.T) {
	secp256k1.Init()
	defer secp256k1.Destroy()

	// digest := make([]byte, 32)
	//	seckey := make([]byte, 32)
	digest, err := hex.DecodeString("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	if err != nil {
		panic(err)
	}

	seckey, err := secp256k1.NewPrivateKeyFromHex("99870ba61ad4bfae18a1c4cea5a6b48882b95633421b108497a2b53dc779a639")
	if err != nil {
		panic(err)
	}

	start := time.Now()

	signature, err := secp256k1.Sign(digest, seckey)
	if err != nil {
		panic(err)
	}
	log.Println("++++++signature:", signature.String())

	{
		pubKey, _ := secp256k1.Recover(digest, signature)
		log.Println("++++++pubKeyBase58:", pubKey.String())
	}

	{
		pubkey, _ := secp256k1.GetPublicKey(seckey)
		log.Println("++++++pub key:", pubkey.String())
		duration := time.Since(start)
		log.Println("++++++duration:", duration)
	}
}

func TestPackTransaction(t *testing.T) {
	tx := `{"expiration":"2021-08-31T05:59:39","ref_block_num":56745,"ref_block_prefix":3729394962,"max_net_usage_words":0,"max_cpu_usage_ms":0,"delay_sec":0,"context_free_actions":[],"actions":[{"account":"eosio.token","name":"transfer","authorization":[{"actor":"helloworld11","permission":"active"}],"data":"10428a97721aa36a0000000000000e3d102700000000000004454f53000000001a7472616e736665722066726f6d20616c69636520746f20626f62"}],"transaction_extensions":[],"signatures":["SIG_K1_KbSF8BCNVA95KzR1qLmdn4VnxRoLVFQ1fZ8VV5gVdW1hLfGBdcwEc93hF7FBkWZip1tq2Ps27UZxceaR3hYwAjKL7j59q8"],"context_free_data":[]}`
	o := orderedmap.New()
	json.Unmarshal([]byte(tx), &o)
	s, _ := json.Marshal(o)
	fmt.Println(string(s))
	for _, k := range o.Keys() {
		v, exist := o.Get(k)
		fmt.Println("+++++++k, v:", exist, k, v)
	}
}

func TestAbi(t *testing.T) {
	serializer := GetABISerializer()
	fieldType := serializer.GetType("transaction", "expiration")
	t.Log("+++++++fieldType:", fieldType)

	fieldType = serializer.GetType("transaction", "transaction_extensions")
	t.Log("+++++++fieldType:", fieldType)

	strAbi, err := os.ReadFile("data/eosio.token.abi")
	if err != nil {
		panic(err)
	}
	serializer.AddContractAbi("hello", strAbi)
	actionStruct := serializer.GetActionStructName("hello", "transfer")
	t.Log("+++++++actionType:", actionStruct)

	args := `{"from": "hello", "to": "alice", "quantity": "1.0000 EOS", "memo": "transfer from alice"}`
	buf, err := serializer.PackAbiStructByName("hello", "transfer", args)
	if err != nil {
		panic(err)
	}
	t.Log("+++++++buf:", hex.EncodeToString(buf))
}

func TestTx(t *testing.T) {
	secp256k1.Init()
	defer secp256k1.Destroy()
	rpc := NewRpc("https://testnode.uuos.network:8443")

	chainInfo, err := rpc.GetInfo()
	if err != nil {
		panic(err)
	}

	//tx := &Transaction{}
	expiration := int(time.Now().Unix()) + 60
	tx := NewTransaction(expiration)
	tx.SetReferenceBlock(chainInfo.LastIrreversibleBlockID)

	pub := "EOS6AjF6hvF7GSuSd4sCgfPKq5uWaXvGM2aQtEUCwmEHygQaqxBSV"
	priv := "5JRYimgLBrRLCBAcjHUWCYRv3asNedTYYzVgmiU4q2ZVxMBiJXL"
	GetWallet().Import("test", "5JRYimgLBrRLCBAcjHUWCYRv3asNedTYYzVgmiU4q2ZVxMBiJXL")
	privKey, err := secp256k1.NewPrivateKeyFromBase58(priv)
	if err != nil {
		panic(err)
	}
	pubKey, err := secp256k1.GetPublicKey(privKey)
	if err != nil {
		panic(err)
	}
	t.Log("+++++++pubKey:", pubKey.String())
	action := NewAction(NewName("eosio.token"),
		NewName("transfer"),
		NewName("helloworld11"),
		NewName("eosio.token"),
		NewAsset(1000, NewSymbol("EOS", 4)),
		"transfer from alice")
	action.AddPermission(NewName("helloworld11"), NewName("active"))
	tx.AddAction(action)

	chainId := chainInfo.ChainID
	sign, err := tx.Sign(priv, chainId)
	if err != nil {
		panic(err)
	}
	t.Log("++++++sign:", sign)

	packedTx := NewPackedtransaction(tx)
	err = packedTx.SignByPrivateKey(priv, chainId)
	if err != nil {
		panic(err)
	}
	t.Log(packedTx.String())

	err = packedTx.Sign(pub, chainId)
	if err != nil {
		panic(err)
	}
	t.Log(packedTx.String())

	r, err := rpc.PushTransaction(packedTx)
	if err != nil {
		panic(err)
	}

	{
		v, ok := DeepGet(r, "processed", "action_traces", 0, "action_ordinal")
		if !ok {
			panic("id not found")
		}
		t.Logf("%T\n", v)
	}

	// rr, err := json.MarshalIndent(r, "", " ")
	// if err != nil {
	// 	panic(err)
	// }

	// t.Log(string(rr))
}

func TestIsoTime(tt *testing.T) {
	// convert iso-8601 into rfc-3339 format
	rfc3339t := strings.Replace("2015-12-23 00:00:00", " ", "T", 1) + "Z"

	rfc3339t = "2021-08-31T05:59:39" + "Z"

	// parse rfc-3339 datetime
	t, err := time.Parse(time.RFC3339, rfc3339t)
	if err != nil {
		panic(err)
	}

	// convert into unix time
	ut := t.UnixNano() / int64(time.Millisecond)

	fmt.Println(ut)
}

func TestRpc(t *testing.T) {
	rpc := NewRpc("http://www.google.com")
	info, err := rpc.GetInfo()
	if err != nil {
		panic(err)
	}
	r, err := json.MarshalIndent(info, "", " ")
	if err != nil {
		panic(err)
	}
	t.Log(string(r))
}