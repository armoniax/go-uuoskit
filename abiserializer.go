package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/orderedmap"
	"github.com/learnforpractice/go-secp256k1/secp256k1"
)

type ABITable struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	IndexType string   `json:"index_type"`
	KeyNames  []string `json:"key_names"`
	KeyTypes  []string `json:"key_types"`
}

type ABIAction struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	RicardianContract string `json:"ricardian_contract"`
}

type ABIStructField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ABIStruct struct {
	Name   string           `json:"name"`
	Base   string           `json:"base"`
	Fields []ABIStructField `json:"fields"`
}

type ABI struct {
	Version          string        `json:"version"`
	Structs          []ABIStruct   `json:"structs"`
	Types            []string      `json:"types"`
	Actions          []ABIAction   `json:"actions"`
	Tables           []ABITable    `json:"tables"`
	RicardianClauses []interface{} `json:"ricardian_clauses"`
	Variants         []interface{} `json:"variants"`
	AbiExtensions    []interface{} `json:"abi_extensions"`
	ErrorMessages    []interface{} `json:"error_messages"`
}

type AbiValue struct {
	value interface{}
}

func (b *AbiValue) UnmarshalJSON(data []byte) error {
	// fmt.Println("+++++:UnmarshaJSON", string(data))
	if data[0] == '{' {
		m := make(map[string]AbiValue)
		err := json.Unmarshal(data, &m)
		if err != nil {
			return err
		}
		b.value = m
	} else if data[0] == '[' {
		m := make([]AbiValue, 0, 1)
		err := json.Unmarshal(data, &m)
		if err != nil {
			return err
		}
		b.value = m
	} else {
		b.value = string(data)
	}
	return nil
}

type ABISerializer struct {
	abiMap         map[string]*ABIStruct
	baseTypeMap    map[string]bool
	contractAbiMap map[string]*ABI
	enc            *Encoder
	dec            *Decoder
	contractName   string
}

var (
	gSerializer *ABISerializer = nil
)

func GetABISerializer() *ABISerializer {
	if gSerializer != nil {
		return gSerializer
	}

	gSerializer = &ABISerializer{}
	gSerializer.enc = NewEncoder(1024 * 1024)

	gSerializer.abiMap = make(map[string]*ABIStruct)
	gSerializer.baseTypeMap = make(map[string]bool)
	gSerializer.contractAbiMap = make(map[string]*ABI)

	gSerializer.AddContractABI("eosio.token", []byte(eosioTokenAbi))

	for _, typeName := range baseTypes {
		gSerializer.baseTypeMap[typeName] = true
	}

	abi := ABI{}
	err := json.Unmarshal([]byte(baseABI), &abi)
	if err != nil {
		panic(err)
	}

	for i := range abi.Structs {
		s := &abi.Structs[i]
		gSerializer.abiMap[s.Name] = s
	}

	return gSerializer
}

func (t *ABISerializer) GetType(structName string, fieldName string) string {
	s := t.abiMap[structName]
	for _, f := range s.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	if s.Base != "" {
		return t.GetType(s.Base, fieldName)
	}
	return ""
}

func (t *ABISerializer) AddContractABI(contractName string, abi []byte) error {
	abiObj := &ABI{}
	err := json.Unmarshal(abi, abiObj)
	if err != nil {
		return err
	}

	t.contractAbiMap[contractName] = abiObj
	return nil
}

func (t *ABISerializer) PackActionArgs(contractName, actionName string, args []byte) ([]byte, error) {
	t.contractName = contractName

	t.enc.buf.Reset()

	m := make(map[string]AbiValue)
	err := json.Unmarshal(args, &m)
	if err != nil {
		return nil, err
	}

	abiStruct := t.GetActionStruct(contractName, actionName)
	if abiStruct == nil {
		return nil, fmt.Errorf("abi struct not found for %s::%s", contractName, actionName)
	}
	t.PackAbiStruct(contractName, abiStruct, m)
	bs := t.enc.buf.Bytes()
	t.enc.buf.Reset()
	ret := make([]byte, len(bs))
	copy(ret, bs)
	return ret, nil
}

func (t *ABISerializer) UnpackActionArgs(contractName string, actionName string, packedValue []byte) ([]byte, error) {
	abiStruct := t.GetActionStruct(contractName, actionName)
	if abiStruct == nil {
		return nil, fmt.Errorf("unknown action %s::%s", contractName, actionName)
	}

	result, err := t.UnpackAbiStruct(contractName, abiStruct, packedValue)
	if result == nil {
		return nil, err
	}
	bs, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func (t *ABISerializer) PackAbiStructByName(contractName string, structName string, args string) ([]byte, error) {
	t.enc.buf.Reset()
	t.contractName = contractName
	m := make(map[string]AbiValue)
	err := json.Unmarshal([]byte(args), &m)
	if err != nil {
		return nil, err
	}

	s := t.GetAbiStruct(contractName, structName)
	err = t.PackAbiStruct(contractName, s, m)
	if err != nil {
		return nil, err
	}
	return t.enc.buf.Bytes(), nil
}

func (t *ABISerializer) PackAbiType(contractName, abiType string, args []byte) ([]byte, error) {
	t.contractName = contractName

	t.enc.buf.Reset()

	m := make(map[string]AbiValue)
	err := json.Unmarshal(args, &m)
	if err != nil {
		return nil, err
	}

	abiStruct := t.GetAbiStruct(contractName, abiType)
	if abiStruct == nil {
		return nil, fmt.Errorf("abi struct not found for %s::%s", contractName, abiType)
	}
	t.PackAbiStruct(contractName, abiStruct, m)
	bs := t.enc.buf.Bytes()
	t.enc.buf.Reset()
	ret := make([]byte, len(bs))
	copy(ret, bs)
	return ret, nil
}

func (t *ABISerializer) UnpackAbiType(contractName string, abiName string, packedValue []byte) ([]byte, error) {
	abiStruct := t.GetAbiStruct(contractName, abiName)
	if abiStruct == nil {
		return nil, fmt.Errorf("unknown action %s::%s", contractName, abiName)
	}

	result, err := t.UnpackAbiStruct(contractName, abiStruct, packedValue)
	if result == nil {
		return nil, err
	}
	bs, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

func StringToInt(s string) (int, error) {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return i, err
}

func StripString(v string) (string, bool) {
	if len(v) < 2 {
		return "", false
	}
	if v[0] != '"' || v[len(v)-1] != '"' {
		return "", false
	}
	return v[1 : len(v)-1], true
}

func IsSymbolValid(sym string) bool {
	if len(sym) > 7 || len(sym) <= 0 {
		return false
	}
	_vv := []byte(sym)
	i := 0
	for ; i < len(_vv); i++ {
		c := _vv[i]
		if c >= byte('A') && c <= byte('Z') {
		} else if c == 0 {
			for ; i < len(_vv); i++ {
				if _vv[i] != 0 {
					return false
				}
			}
		} else {
			return false
		}
	}
	return true
}

func (t *ABISerializer) PackAbiValue(typ string, value AbiValue) error {
	v := value.value.(string)
	return t.ParseAbiStringValue(typ, v)
}

//{"quantity":"1.0000 EOS","contract":"eosio.token"}
type AbiExtendedAsset struct {
	Quantity string `json:"quantity"`
	Contract string `json:"contract"`
}

func (t *ABISerializer) ParseAbiStringValue(typ string, v string) error {
	switch typ {
	case "bool":
		if v == "true" {
			t.enc.PackBool(true)
		} else if v == "false" {
			t.enc.PackBool(false)
		} else {
			return fmt.Errorf("invalid bool value: %s", v)
		}
		break
	case "int8":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxInt8 || n < math.MinInt8 {
			return fmt.Errorf("int8 overflow: %d", n)
		}
		t.enc.PackInt8(int8(n))
		break
	case "uint8":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxUint8 || n < 0 {
			return fmt.Errorf("uint8 overflow: %d", n)
		}
		t.enc.PackUint8(uint8(n))
		break
	case "int16":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxInt16 || n < math.MinInt16 {
			return fmt.Errorf("int16 overflow: %d", n)
		}
		t.enc.PackInt16(int16(n))
		break
	case "uint16":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxUint16 || n < 0 {
			return fmt.Errorf("uint16 overflow: %d", n)
		}
		t.enc.PackUint16(uint16(n))
		break
	case "int32":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxInt32 || n < math.MinInt32 {
			return fmt.Errorf("int32 overflow: %d", n)
		}
		t.enc.PackInt32(int32(n))
	case "uint32":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxUint32 || n < 0 {
			return fmt.Errorf("uint32 overflow: %d", n)
		}
		t.enc.PackUint32(uint32(n))
	case "int64":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxInt64 || n < math.MinInt64 {
			return fmt.Errorf("int64 overflow: %d", n)
		}
		break
	case "uint64":
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return err
		}
		if n > math.MaxUint64 || n < 0 {
			return fmt.Errorf("uint64 overflow: %d", n)
		}
		t.enc.PackUint64(n)
	case "int128", "uint128", "float128":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid int128 value: %s", v)
		}
		if v[:2] != "0x" {
			return fmt.Errorf("invalid int128 value: %s", v)
		}
		v = v[2:]
		bs, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		if len(bs) > 16 {
			return fmt.Errorf("invalid int128 value: %s", v)
		}
		buf := make([]byte, 16)
		copy(buf[:], bs)
		t.enc.WriteBytes(buf)
	case "varint32":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxInt32 || n < math.MinInt32 {
			return fmt.Errorf("varint32 overflow: %d", n)
		}
		t.enc.PackVarInt32(int32(n))
		break
	case "varuint32":
		n, err := StringToInt(v)
		if err != nil {
			return err
		}
		if n > math.MaxUint32 || n < 0 {
			return fmt.Errorf("varuint32 overflow: %d", n)
		}
		t.enc.PackVarUint32(uint32(n))
	case "float32":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid float32 value: %s", v)
		}
		n, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return err
		}
		t.enc.PackFloat32(float32(n))
	case "float64":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid float64 value: %s", v)
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		t.enc.PackFloat64(n)
	case "time_point":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid time_point value: %s", v)
		}
		tt, err := time.Parse(time.RFC3339, string(v)+"Z")
		if err != nil {
			return err
		}
		n := tt.UnixNano()
		t.enc.PackInt64(n)
	case "time_point_sec", "block_timestamp_type":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid time_point_sec value: %s", v)
		}
		tt, err := time.Parse(time.RFC3339, string(v)+"Z")
		if err != nil {
			return err
		}
		n := tt.Unix()
		t.enc.PackUint32(uint32(n))
	case "name":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid name value: %s", v)
		}
		n := S2N(v)
		if N2S(n) != v {
			return fmt.Errorf("invalid name value: %s", v)
		}
		t.enc.PackUint64(n)
	case "bytes":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid bytes value: %s", v)
		}
		bs, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		t.enc.PackBytes(bs)
	case "string":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid string value: %s", v)
		}
		t.enc.PackString(v)
	case "checksum160":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid checksum160 value: %s", v)
		}
		bs, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		if len(bs) > 20 {
			return fmt.Errorf("invalid checksum160 value: %s", v)
		}
		buf := make([]byte, 20)
		copy(buf[:], bs)
		t.enc.WriteBytes(buf)
	case "checksum256":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid checksum256 value: %s", v)
		}
		bs, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		if len(bs) > 32 {
			return fmt.Errorf("invalid checksum256 value: %s", v)
		}
		buf := make([]byte, 32)
		copy(buf[:], bs)
		t.enc.WriteBytes(buf)
	case "checksum512":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid checksum512 value: %s", v)
		}
		bs, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		if len(bs) > 64 {
			return fmt.Errorf("invalid checksum512 value: %s", v)
		}
		buf := make([]byte, 64)
		copy(buf[:], bs)
		t.enc.WriteBytes(buf)
	case "public_key":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid public_key value: %s", v)
		}
		pub, err := secp256k1.PublicKeyFromBase58(v)
		if err != nil {
			return err
		}
		t.enc.WriteBytes([]byte{0})
		t.enc.WriteBytes(pub.Data[:])
		break
	case "signature":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid signature value: %s", v)
		}
		sig, err := secp256k1.NewSignatureFromBase58(v)
		if err != nil {
			return err
		}
		t.enc.WriteBytes([]byte{0})
		t.enc.WriteBytes(sig.Data[:])
	case "symbol":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid symbol value: %s", v)
		}
		vv := strings.Split(v, ",")
		if len(vv) != 2 {
			return fmt.Errorf("invalid symbol value: %s", v)
		}
		n, err := strconv.ParseUint(vv[0], 10, 64)
		if err != nil {
			return err
		}
		if n > 16 {
			return fmt.Errorf("invalid symbol value: %s", v)
		}
		if len(vv[1]) > 7 || len(vv[1]) <= 0 {
			return fmt.Errorf("invalid symbol value: %s", v)
		}
		if !IsSymbolValid(vv[1]) {
			return fmt.Errorf("invalid symbol value: %s", v)
		}

		_vv := []byte(vv[1])
		symbol_code := make([]byte, 7)
		copy(symbol_code[:], _vv[:])
		t.enc.WriteBytes(symbol_code)
		break
	case "symbol_code":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid symbol_code value: %s", v)
		}
		if !IsSymbolValid(v) {
			return fmt.Errorf("invalid symbol_code value: %s", v)
		}
		symbol_code := make([]byte, 8)
		copy(symbol_code[:], []byte(v))
		t.enc.WriteBytes(symbol_code)
	case "asset":
		v, ok := StripString(v)
		if !ok {
			return fmt.Errorf("invalid asset value: %s", v)
		}
		r, ok := ParseAsset(v)
		if !ok {
			return fmt.Errorf("invalid asset value: %s", v)
		}
		t.enc.WriteBytes(r)
	case "extended_asset":
		a := AbiExtendedAsset{}
		err := json.Unmarshal([]byte(v), &a)
		if err != nil {
			return err
		}
		r, ok := ParseAsset(a.Quantity)
		if !ok {
			return fmt.Errorf("invalid asset value: %s", v)
		}
		t.enc.WriteBytes(r)

		n := S2N(a.Contract)
		if N2S(n) != v {
			return fmt.Errorf("invalid name value: %s", v)
		}
		t.enc.PackUint64(n)
	default:
		return fmt.Errorf("unknown type %s", typ)
	}

	return nil
}

func (t *ABISerializer) unpackAbiStructField(typ string) (interface{}, error) {
	switch typ {
	case "bool":
		v, err := t.dec.UnpackBool()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "int8":
		v, err := t.dec.UnpackInt8()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "uint8":
		v, err := t.dec.UnpackUint8()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "int16":
		v, err := t.dec.UnpackInt16()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "uint16":
		v, err := t.dec.UnpackUint16()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "int32":
		v, err := t.dec.UnpackInt32()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "uint32":
		v, err := t.dec.UnpackUint32()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "int64":
		v, err := t.dec.UnpackInt64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "uint64":
		v, err := t.dec.UnpackUint64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "int128", "uint128", "float128":
		buf := [16]byte{}
		err := t.dec.Read(buf[:])
		if err != nil {
			return nil, err
		}
		return "0x" + hex.EncodeToString(buf[:]), nil
	case "varint32":
		v, err := t.dec.UnpackVarInt32()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "varuint32":
		v, err := t.dec.UnpackVarUint32()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "float32":
		v, err := t.dec.UnpackFloat32()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "float64":
		v, err := t.dec.UnpackFloat64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "time_point":
		v, err := t.dec.ReadUint64()
		if err != nil {
			return nil, err
		}
		//convert seconds to iso8601
		return time.Unix(0, int64(v)).Format("2006-01-02T15:04:05"), nil
	case "time_point_sec", "block_timestamp_type":
		v, err := t.dec.ReadUint32()
		if err != nil {
			return nil, err
		}
		//convert seconds to iso8601
		return time.Unix(int64(v), 0).Format("2006-01-02T15:04:05"), nil
	case "name":
		v, err := t.dec.ReadUint64()
		if err != nil {
			return nil, err
		}
		return N2S(v), nil
	case "bytes":
		v, err := t.dec.UnpackBytes()
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(v), nil
	case "string":
		v, err := t.dec.UnpackString()
		if err != nil {
			return nil, err
		}
		return v, nil
	case "checksum160":
		v := make([]byte, 20)
		err := t.dec.Read(v)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(v), nil
	case "checksum256":
		v := make([]byte, 32)
		err := t.dec.Read(v)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(v), nil
	case "checksum512":
		v := make([]byte, 64)
		err := t.dec.Read(v)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(v), nil
	case "public_key":
		v := make([]byte, 34)
		err := t.dec.Read(v)
		if err != nil {
			return nil, err
		}
		pub := secp256k1.PublicKey{}
		copy(pub.Data[:], v[1:])
		return pub.String(), nil
	case "signature":
		v := make([]byte, 66)
		err := t.dec.Read(v)
		if err != nil {
			return nil, err
		}
		sig := secp256k1.Signature{}
		copy(sig.Data[:], v[1:])
		return sig.String(), nil
	case "symbol":
		buf := make([]byte, 8)
		err := t.dec.Read(buf)
		if err != nil {
			return nil, err
		}
		precision := int(buf[0])
		sym := string(buf[1:])
		sym = strings.TrimRight(sym, "\x00")
		return fmt.Sprintf("%s,%d", sym, precision), nil
	case "symbol_code":
		buf := make([]byte, 8)
		err := t.dec.Read(buf)
		if err != nil {
			return nil, err
		}
		sym := string(buf)
		sym = strings.TrimRight(sym, "\x00")
		return sym, nil
	case "asset":
		amount, err := t.dec.UnpackInt64()
		if err != nil {
			return nil, err
		}
		sym := make([]byte, 8)
		err = t.dec.Read(sym)
		if err != nil {
			return nil, err
		}
		precision := int64(sym[0])
		_sym := strings.TrimRight(string(sym[1:]), "\x00")
		format := fmt.Sprintf("%%d.%%0%dd %%s", precision)
		precision = int64(math.Pow10(int(precision)))
		return fmt.Sprintf(format, amount/precision, amount%precision, _sym), nil
	case "extended_asset":
		// {"quantity":"1.0000 EOS","contract":"eosio.token"}
		quantity, err := t.unpackAbiStructField("asset")
		if err != nil {
			return nil, err
		}
		contract, err := t.unpackAbiStructField("name")
		if err != nil {
			return nil, err
		}
		m := orderedmap.New()
		m.Set("quantity", quantity)
		m.Set("contract", contract)
		return m, nil
	default:
		return nil, fmt.Errorf("unknown type %s", typ)
	}
}

func ParseAsset(v string) ([]byte, bool) {
	vv := strings.Split(v, " ")
	if len(vv) != 2 {
		return nil, false
	}
	amount := vv[0]
	symbol_code := vv[1]
	if !IsSymbolValid(symbol_code) {
		return nil, false
	}
	_, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return nil, false
	}
	_amount := strings.Split(amount, ".")
	if len(_amount) != 2 {
		return nil, false
	}

	n, err := strconv.Atoi(_amount[1])
	if err != nil {
		return nil, false
	}
	if n != 0 {
		return nil, false
	}
	precision := len(_amount[1])
	__amount, err := strconv.Atoi(_amount[0])
	if err != nil {
		return nil, false
	}
	if __amount < 0 || __amount > math.MaxInt64 {
		return nil, false
	}

	amount = strings.Replace(amount, ".", "", 1)
	nAmount, err := strconv.ParseInt(amount, 10, 64)
	if err != nil {
		return nil, false
	}
	enc := NewEncoder(8)
	enc.PackInt64(nAmount)

	enc.WriteBytes([]byte{byte(precision)})
	_symbol_code := make([]byte, 7)
	copy(_symbol_code[:], []byte(symbol_code)[:])
	enc.WriteBytes(_symbol_code)
	return enc.GetBytes(), true
}

//TODO:
func (t *ABISerializer) PackArrayAbiValue(typ string, value []AbiValue) error {
	return nil
}

func (t *ABISerializer) PackAbiStruct(contractName string, abiStruct *ABIStruct, m map[string]AbiValue) error {
	for _, v := range abiStruct.Fields {
		typ := v.Type
		name := v.Name
		abiValue, ok := m[name]
		if !ok {
			return fmt.Errorf("missing field %s", name)
		}
		err := t.ParseAbiValue(typ, abiValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *ABISerializer) UnpackAbiStruct(contractName string, abiStruct *ABIStruct, packedValue []byte) (*orderedmap.OrderedMap, error) {
	t.dec = NewDecoder(packedValue)
	result := orderedmap.New()
	err := t.unpackAbiStruct(contractName, abiStruct, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *ABISerializer) unpackAbiStruct(contractName string, abiStruct *ABIStruct, result *orderedmap.OrderedMap) error {
	for _, v := range abiStruct.Fields {
		typ := v.Type
		name := v.Name

		//try to unpack inner abi type
		if _, ok := t.baseTypeMap[typ]; ok {
			v, err := t.unpackAbiStructField(typ)
			if err == nil {
				result.Set(name, v)
				continue
			}
		}

		//try to unpack Abi struct
		subStruct := t.GetAbiStruct(contractName, typ)
		if subStruct != nil {
			subResult := orderedmap.New()
			result.Set(name, subResult)
			err := t.unpackAbiStruct(contractName, subStruct, subResult)
			if err != nil {
				return err
			}
			continue
		}

		//try to unpack array
		if !strings.HasSuffix(typ, "[]") {
			return fmt.Errorf("unknown type %s", typ)
		}
		typ = strings.TrimSuffix(typ, "[]")
		arr := make([]interface{}, 0)
		count, err := t.dec.UnpackLength()
		if err != nil {
			return err
		}
		if _, ok := t.baseTypeMap[typ]; ok {
			for i := 0; i < count; i++ {
				v, err := t.unpackAbiStructField(typ)
				if err != nil {
					return err
				}
				arr = append(arr, v)
			}
			result.Set(name, arr)
			continue
		}

		subStruct = t.GetAbiStruct(contractName, typ)
		if subStruct == nil {
			return fmt.Errorf("unknown type %s", typ)
		}
		for i := 0; i < count; i++ {
			subResult := orderedmap.New()
			err := t.unpackAbiStruct(contractName, subStruct, subResult)
			if err != nil {
				return err
			}
			arr = append(arr, subResult)
		}
		result.Set(name, arr)
	}
	return nil
}

func (t *ABISerializer) ParseAbiValue(typ string, abiValue AbiValue) error {
	switch v := abiValue.value.(type) {
	case string:
		err := t.ParseAbiStringValue(typ, v)
		if err != nil {
			return err
		}
	case AbiValue:
		err := t.ParseAbiValue(typ, v)
		if err != nil {
			return err
		}
	case []AbiValue:
		err := t.PackArrayAbiValue(typ, v)
		if err != nil {
			return err
		}
	case map[string]AbiValue:
		s := t.GetAbiStruct(t.contractName, typ)
		if s == nil {
			return fmt.Errorf("Unknown ABI type %s", typ)
		}
		err := t.PackAbiStruct(t.contractName, s, v)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported type %[1]T: %[1]s", v)
	}
	return nil
}

func (t *ABISerializer) GetAbiStruct(contractName string, structName string) *ABIStruct {
	abi := t.contractAbiMap[contractName]
	for j := range abi.Structs {
		s := &abi.Structs[j]
		if s.Name == structName {
			return s
		}
	}
	s, ok := t.abiMap[structName]
	if !ok {
		return nil
	}
	return s
}

func (t *ABISerializer) GetActionStruct(contractName string, actionName string) *ABIStruct {
	abi, ok := t.contractAbiMap[contractName]
	if !ok {
		return nil
	}

	if abi.Actions == nil {
		return nil
	}

	for i := range abi.Actions {
		action := &abi.Actions[i]
		if action.Name == actionName {
			for j := range abi.Structs {
				s := &abi.Structs[j]
				if s.Name == action.Type {
					return s
				}
			}
		}
	}
	return nil
}

func (t *ABISerializer) GetActionStructName(contractName string, actionName string) string {
	abi := t.contractAbiMap[contractName]
	for i := range abi.Actions {
		action := &abi.Actions[i]
		if action.Name == actionName {
			return action.Type
		}
	}
	return ""
}
