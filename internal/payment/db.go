package payment

import (
	"database/sql"
	"log"
)

// Сохранение транзакции в БД
func InsertTransaction(db *sql.DB, trx *Transaction) (int64, error) {
	var id int64
	err := db.QueryRow(`INSERT INTO transactions (user_id, amount, status, url, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW()) RETURNING id`,
		trx.TelegramUserID, trx.Amount, trx.Status, trx.URL).Scan(&id)
	return id, err
}

// Получение транзакции по charge_id (TelegramPaymentChargeID)
func GetTransactionByChargeID(db *sql.DB, chargeID string) (*Transaction, error) {
	row := db.QueryRow(`SELECT id, telegram_payment_charge_id, user_id, amount, invoice_payload, status, type, reason, url, created_at, updated_at FROM transactions WHERE telegram_payment_charge_id = $1`, chargeID)
	var t Transaction
	var createdAt, updatedAt string
	var telegramPaymentChargeID, invoicePayload, typeField, reason sql.NullString
	err := row.Scan(&t.ID, &telegramPaymentChargeID, &t.TelegramUserID, &t.Amount, &invoicePayload, &t.Status, &typeField, &reason, &t.URL, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if telegramPaymentChargeID.Valid {
		t.TelegramPaymentChargeID = telegramPaymentChargeID.String
	}
	if invoicePayload.Valid {
		t.InvoicePayload = invoicePayload.String
	}
	if typeField.Valid {
		t.Type = typeField.String
	}
	if reason.Valid {
		t.Reason = reason.String
	}
	return &t, nil
}

// Получение транзакции по id
func GetTransactionByID(db *sql.DB, id int64) (*Transaction, error) {
	row := db.QueryRow(`SELECT id, telegram_payment_charge_id, user_id, amount, invoice_payload, status, type, reason, url, created_at, updated_at FROM transactions WHERE id = $1`, id)
	var t Transaction
	var createdAt, updatedAt string
	var telegramPaymentChargeID, invoicePayload, typeField, reason sql.NullString
	err := row.Scan(&t.ID, &telegramPaymentChargeID, &t.TelegramUserID, &t.Amount, &invoicePayload, &t.Status, &typeField, &reason, &t.URL, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if telegramPaymentChargeID.Valid {
		t.TelegramPaymentChargeID = telegramPaymentChargeID.String
	}
	if invoicePayload.Valid {
		t.InvoicePayload = invoicePayload.String
	}
	if typeField.Valid {
		t.Type = typeField.String
	}
	if reason.Valid {
		t.Reason = reason.String
	}
	return &t, nil
}

// Получение всех транзакций из БД
func GetAllTransactionsFromDB(db *sql.DB) ([]Transaction, error) {
	log.Printf("[DB] Запрашиваем все транзакции из БД")
	rows, err := db.Query(`SELECT id, telegram_payment_charge_id, user_id, amount, invoice_payload, status, type, reason, url, created_at, updated_at FROM transactions`)
	if err != nil {
		log.Printf("[DB] Ошибка запроса всех транзакций: %v", err)
		return nil, err
	}
	defer rows.Close()
	var result []Transaction
	count := 0
	for rows.Next() {
		var t Transaction
		var createdAt, updatedAt string
		var telegramPaymentChargeID, invoicePayload, typeField, reason sql.NullString
		err := rows.Scan(&t.ID, &telegramPaymentChargeID, &t.TelegramUserID, &t.Amount, &invoicePayload, &t.Status, &typeField, &reason, &t.URL, &createdAt, &updatedAt)
		if err != nil {
			log.Printf("[DB] Ошибка сканирования строки %d: %v", count, err)
			continue
		}
		if telegramPaymentChargeID.Valid {
			t.TelegramPaymentChargeID = telegramPaymentChargeID.String
		}
		if invoicePayload.Valid {
			t.InvoicePayload = invoicePayload.String
		}
		if typeField.Valid {
			t.Type = typeField.String
		}
		if reason.Valid {
			t.Reason = reason.String
		}
		result = append(result, t)
		count++
		log.Printf("[DB] Найдена транзакция: id=%d, user_id=%d, status=%s, url=%s", t.ID, t.TelegramUserID, t.Status, t.URL)
	}
	log.Printf("[DB] Всего найдено транзакций: %d", count)
	return result, nil
}

// Создание транзакции со статусом 'pending' и возврат id
func CreatePendingTransaction(db *sql.DB, userID int64, amount int, url string) (int64, error) {
	log.Printf("[DB] Создаём pending транзакцию: user_id=%d, amount=%d, url=%s", userID, amount, url)
	var id int64
	err := db.QueryRow(`INSERT INTO transactions (user_id, amount, status, url, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW()) RETURNING id`,
		userID, amount, "pending", url).Scan(&id)
	if err != nil {
		log.Printf("[DB] Ошибка создания pending транзакции: %v", err)
		return 0, err
	}
	log.Printf("[DB] Pending транзакция создана с id=%d", id)
	return id, err
}

// Обновление транзакции после оплаты: charge_id и статус
func UpdateTransactionAfterPayment(db *sql.DB, id int64, chargeID string, status string) error {
	_, err := db.Exec(`UPDATE transactions SET status = $1, telegram_payment_charge_id = $2, updated_at = NOW() WHERE id = $3`, status, chargeID, id)
	return err
}
