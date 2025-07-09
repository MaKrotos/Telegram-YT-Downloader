package payment

import (
	"log"
)

type Transaction struct {
	TelegramPaymentChargeID string
	TelegramUserID          int64
	Amount                  int
	InvoicePayload          string
	Status                  string
	Type                    string
	Reason                  string
}

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

// Заглушка для возврата средств через Telegram Stars
func RefundStarPayment(userID int64, telegramPaymentChargeID string, amount int, reason string) error {
	log.Printf("[RefundStarPayment] Возврат %d XTR пользователю %d, charge_id=%s, причина: %s", amount, userID, telegramPaymentChargeID, reason)
	// Здесь должна быть интеграция с Telegram API для возврата средств
	return nil
}
