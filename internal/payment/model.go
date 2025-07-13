package payment

type Transaction struct {
	ID                      int64 // Новое поле для id из БД
	TelegramPaymentChargeID string
	TelegramUserID          int64
	Amount                  int
	InvoicePayload          string
	Status                  string
	Type                    string
	Reason                  string
	URL                     string // Новое поле для ссылки
}
