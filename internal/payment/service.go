package payment

import "log"

type TransactionService struct {
	transactions []Transaction
}

func NewTransactionService() *TransactionService {
	return &TransactionService{transactions: []Transaction{}}
}

func (s *TransactionService) AddTransaction(trx *Transaction) error {
	s.transactions = append(s.transactions, *trx)
	log.Printf("[TransactionService] Записана транзакция: %+v", trx)
	return nil
}

func (s *TransactionService) GetAllTransactions() []Transaction {
	return s.transactions
}

func (s *TransactionService) MarkRefunded(chargeID string) {
	for i, trx := range s.transactions {
		if trx.TelegramPaymentChargeID == chargeID && trx.Status == "success" {
			s.transactions[i].Status = "refunded"
		}
	}
}
