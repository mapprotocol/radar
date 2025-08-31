package memo

import (
	"fmt"
	"strings"
)

type CrossOutMemo struct {
	ToChain      string
	OrderId      string
	Amount       string // no decimal
	DstToken     string
	Sender       string
	MinOutAmount string
	Receiver     string
	Affiliate    string
}

func NewCrossOutMemo(toChain, orderId, amount, dstToken, sender, minOutAmount, receiver, affiliate string) *CrossOutMemo {
	return &CrossOutMemo{
		ToChain:      toChain,
		OrderId:      orderId,
		Amount:       amount,
		DstToken:     dstToken,
		Sender:       sender,
		MinOutAmount: minOutAmount,
		Receiver:     receiver,
		Affiliate:    affiliate,
	}
}

func (m *CrossOutMemo) String() string {
	_type := "Mx"
	args := []string{
		_type,
		m.ToChain,
		m.OrderId,
		m.Amount,
		m.DstToken,
		m.Sender,
		m.MinOutAmount,
		m.Receiver,
		m.Affiliate,
	}
	return strings.Join(args, "|")
}

func (m *CrossOutMemo) FromString(s string) error {
	parts := strings.Split(s, "|")
	if len(parts) != 9 {
		return fmt.Errorf("invalid CrossOutMemo string: %s", s)
	}
	m.ToChain = parts[1]
	m.OrderId = parts[2]
	m.Amount = parts[3]
	m.DstToken = parts[4]
	m.Sender = parts[5]
	m.MinOutAmount = parts[6]
	m.Receiver = parts[7]
	m.Affiliate = parts[8]
	return nil
}
