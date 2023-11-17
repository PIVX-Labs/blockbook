package pivx

import (
	//	"bytes"
	//	"encoding/hex"
	//	"encoding/json"
	//	"io"
	//	"math/big"
	"bytes"
	"fmt"

	"github.com/juju/errors"
	//	"github.com/martinboehm/btcd/blockchain"
	"github.com/martinboehm/btcd/txscript"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	//"github.com/trezor/blockbook/bchain/coins/utils"
	//	"github.com/trezor/blockbook/bchain/coins/utils"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xe9fdc490
	TestnetMagic wire.BitcoinNet = 0xba657645

	// Zerocoin op codes
	OP_ZEROCOINMINT  = 0xc1
	OP_ZEROCOINSPEND = 0xc2

	OP_CHECKCOLDSTAKEVERIFY_LOF = 0xd1
	 OP_CHECKCOLDSTAKEVERIFY = 0xd2
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	// PIVX mainnet Address encoding magics
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{30} // starting with 'D'
	MainNetParams.ScriptHashAddrID = []byte{13}
	MainNetParams.PrivateKeyID = []byte{212}

	// PIVX testnet Address encoding magics
	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{139} // starting with 'x' or 'y'
	TestNetParams.ScriptHashAddrID = []byte{19}
	TestNetParams.PrivateKeyID = []byte{239}
}

// PivXParser handle
type PivXParser struct {
	*btc.BitcoinLikeParser
	baseparser                         *bchain.BaseParser
	BitcoinOutputScriptToAddressesFunc btc.OutputScriptToAddressesFunc
	stakingAddrHashId []byte
}

// NewPivXParser returns new PivXParser instance
func NewPivXParser(params *chaincfg.Params, c *btc.Configuration) *PivXParser {
	p := &PivXParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
	p.BitcoinOutputScriptToAddressesFunc = p.OutputScriptToAddressesFunc
	switch params.Net.String() {
	case "test":
		p.stakingAddrHashId = []byte{63}
	default:
		p.stakingAddrHashId = []byte{73}
	}
	//p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main PivX network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err != nil {
			panic(err)
		}
	}
	fmt.Println(chain);

	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}

func (p *PivXParser) stakePKHToAddress(bytes []byte) (string, error) {
	// Hack: Because btcutils expects a chainCfg obj,
	// We'll temporarely switch the Pubkeyhasid of params
	save := p.Params.PubKeyHashAddrID
	p.Params.PubKeyHashAddrID = p.stakingAddrHashId
	addr, err := btcutil.NewAddressPubKeyHash(bytes, p.Params)
	p.Params.PubKeyHashAddrID = save
	return addr.EncodeAddress(), err
}

func (p *PivXParser) pkhToAddress(bytes []byte) (string, error) {
	addr, err := btcutil.NewAddressPubKeyHash(bytes, p.Params)
	return addr.EncodeAddress(), err
}

// PackTx packs transaction to byte array using protobuf
func (p *PivXParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *PivXParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// GetAddrDescFromAddress returns internal address representation (descriptor) of given address
func (p *PivXParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.BitcoinLikeParser.GetAddrDescFromAddress(address)
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *PivXParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	fmt.Println(addrDesc)
	if len(addrDesc) == 0 {
		return []string{ "Coinbase Tx" }, false, nil
	}
	addr, isSearchable, err := p.parseColdStakeAddress(addrDesc)
	if err != nil {
		return p.BitcoinLikeParser.GetAddressesFromAddrDesc(addrDesc)
	}
	return addr, isSearchable, nil

}


func (p *PivXParser) parseColdStakeAddress(addrDesc bchain.AddressDescriptor) ([]string, bool,
	error) {
	add := bytes.NewReader(addrDesc)
	first_part := []byte {
		txscript.OP_DUP, txscript.OP_HASH160, txscript.OP_ROT, txscript.OP_IF,
	}
	if len(addrDesc) == 51 {
		for _, b := range first_part {
			byte, err := add.ReadByte()
			if err != nil || byte != b {
				return nil, false, errors.New("Invalid cold stake address")
			}
		}

		bb, err := add.ReadByte()
		if err != nil || (bb != OP_CHECKCOLDSTAKEVERIFY && bb != OP_CHECKCOLDSTAKEVERIFY_LOF) {
			return nil, false, errors.New("Invalid cold stake address")
		}
		pkh1 := make([]byte, 21)
		n, err := add.Read(pkh1)
		if n != 21 || err != nil {
			return nil, false, errors.New("Invalid cold stake address")
		}
		bb, err = add.ReadByte()
		if err != nil || bb != txscript.OP_ELSE {
			print(bb)
			return nil, false, errors.New("Invalid cold stake address")
		}
		
		pkh2 := make([]byte, 21)
		n, err = add.Read(pkh2)
		if n != 21 || err != nil {
			return nil, false, errors.New("Invalid cold stake address")
		}

		add1, err := p.stakePKHToAddress(pkh1[1:])
		if err != nil {
			return nil, false, errors.New("Invalid cold stake address")
		}
		add2, err := p.pkhToAddress(pkh2[1:])
		if err != nil {
			return nil, false, errors.New("Invalid cold stake address")
		}
		return []string{ add1, add2 }, true, nil

	}
	return nil, false, errors.New("Invalid cold stake address")
}
