package tx

import (
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/thor"
)

// Transaction is an immutable tx type.
type Transaction struct {
	body body

	cache struct {
		hash *thor.Hash
	}
}

var _ cry.Signable = (*Transaction)(nil)

// body describes details of a tx.
type body struct {
	Clauses     []*Clause
	GasPrice    *big.Int
	Gas         uint64
	Nonce       uint64
	TimeBarrier uint64
	DependsOn   *thor.Hash `rlp:"nil"`
	Signature   []byte
}

// Hash returns hash of tx.
func (t *Transaction) Hash() (hash thor.Hash) {
	if cached := t.cache.hash; cached != nil {
		return *cached
	}

	hw := cry.NewHasher()
	rlp.Encode(hw, t)

	hw.Sum(hash[:0])
	t.cache.hash = &hash
	return hash
}

// SigningHash returns hash of tx excludes signature.
func (t *Transaction) SigningHash() (hash thor.Hash) {
	hw := cry.NewHasher()
	rlp.Encode(hw, []interface{}{
		t.body.Clauses,
		t.body.GasPrice,
		t.body.Gas,
		t.body.Nonce,
		t.body.TimeBarrier,
		t.body.DependsOn,
	})
	hw.Sum(hash[:0])
	return
}

// GasPrice returns gas price.
func (t *Transaction) GasPrice() *big.Int {
	return new(big.Int).Set(t.body.GasPrice)
}

// Gas returns gas provision for this tx.
func (t *Transaction) Gas() uint64 {
	return t.body.Gas
}

// TimeBarrier returns time barrier.
// It's required that tx.TimeBarrier <= block.Timestamp,
// when a tx was packed in a block.
func (t *Transaction) TimeBarrier() uint64 {
	return t.body.TimeBarrier
}

// NewClauseIterator create a clause iteartor.
// It returns a function acts as 'Next'.
func (t *Transaction) NewClauseIterator() func() (clause *Clause, index int, ok bool) {
	i := 0
	return func() (c *Clause, index int, ok bool) {
		if i >= len(t.body.Clauses) {
			return nil, 0, false
		}
		c, index, ok = t.body.Clauses[i], i, true
		i++
		return
	}
}

// ClauseCount returns count of clauses contained in this tx.
func (t *Transaction) ClauseCount() int {
	return len(t.body.Clauses)
}

// Signature returns signature.
func (t *Transaction) Signature() []byte {
	return append([]byte(nil), t.body.Signature...)
}

// WithSignature create a new tx with signature set.
func (t *Transaction) WithSignature(sig []byte) *Transaction {
	newTx := Transaction{
		body: t.body,
	}
	// copy sig
	newTx.body.Signature = append([]byte(nil), sig...)
	return &newTx
}

// EncodeRLP implements rlp.Encoder
func (t *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &t.body)
}

// DecodeRLP implements rlp.Decoder
func (t *Transaction) DecodeRLP(s *rlp.Stream) error {
	var body body
	if err := s.Decode(&body); err != nil {
		return err
	}
	*t = Transaction{
		body: body,
	}
	return nil
}

// IntrinsicGas returns intrinsic gas of tx.
// That's sum of all clauses intrinsic gas.
func (t *Transaction) IntrinsicGas() (uint64, error) {
	clauseCount := len(t.body.Clauses)
	if clauseCount == 0 {
		return params.TxGas, nil
	}

	firstClause := t.body.Clauses[0]
	total := core.IntrinsicGas(firstClause.body.Data, firstClause.body.To == nil, true)

	for _, c := range t.body.Clauses[1:] {
		contractCreation := c.body.To == nil
		total.Add(total, core.IntrinsicGas(c.body.Data, contractCreation, true))

		// sub over-payed gas for clauses after the first one.
		if contractCreation {
			total.Sub(total, new(big.Int).SetUint64(params.TxGasContractCreation-thor.ClauseGasContractCreation))
		} else {
			total.Sub(total, new(big.Int).SetUint64(params.TxGas-thor.ClauseGas))
		}
	}

	if total.BitLen() > 64 {
		return 0, errors.New("intrinsic gas too large")
	}
	return total.Uint64(), nil
}
