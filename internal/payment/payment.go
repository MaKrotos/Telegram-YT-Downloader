package payment

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
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

func (s *TransactionService) MarkRefunded(chargeID string) {
	for i, trx := range s.transactions {
		if trx.TelegramPaymentChargeID == chargeID && trx.Status == "success" {
			s.transactions[i].Status = "refunded"
		}
	}
}

// Заглушка для возврата средств через Telegram Stars
func RefundStarPayment(userID int64, telegramPaymentChargeID string, amount int, reason string) error {
	log.Printf("[RefundStarPayment] Возврат %d XTR пользователю %d, charge_id=%s, причина: %s", amount, userID, telegramPaymentChargeID, reason)
	// Реальный возврат через Bot API
	apiToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if apiToken == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN не задан")
	}
	apiUrl := fmt.Sprintf("https://api.telegram.org/bot%s/refundStarPayment", apiToken)
	data := url.Values{}
	data.Set("user_id", fmt.Sprintf("%d", userID))
	data.Set("telegram_payment_charge_id", telegramPaymentChargeID)
	resp, err := http.PostForm(apiUrl, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Ошибка возврата: %s", string(body))
	}
	log.Printf("[RefundStarPayment] Ответ Telegram: %s", string(body))
	return nil
}
