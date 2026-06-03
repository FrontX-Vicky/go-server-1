package openapi

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

const activeMembersRenewalTable = "pf_TickleRight_9210.active_members_renewal_range"

type dateRange struct {
	start string
	end   string
}

type paginatedRows struct {
	Rows       []orderedRow `json:"rows"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}

func defaultCurrentMonthRange(now time.Time) dateRange {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, -1)
	return dateRange{
		start: start.Format("2006-01-02"),
		end:   end.Format("2006-01-02"),
	}
}

func endOfTwoMonthsAgo(now time.Time) string {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return monthStart.AddDate(0, 0, -1).AddDate(0, -1, 0).Format("2006-01-02")
}

func parseDateRange(startDate string, endDate string) (dateRange, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return dateRange{}, fmt.Errorf("invalid start_date, expected YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return dateRange{}, fmt.Errorf("invalid end_date, expected YYYY-MM-DD")
	}
	if end.Before(start) {
		return dateRange{}, fmt.Errorf("end_date must be on or after start_date")
	}
	return dateRange{start: start.Format("2006-01-02"), end: end.Format("2006-01-02")}, nil
}

func previousMonthRange(current dateRange) (dateRange, error) {
	start, err := time.Parse("2006-01-02", current.start)
	if err != nil {
		return dateRange{}, err
	}
	end, err := time.Parse("2006-01-02", current.end)
	if err != nil {
		return dateRange{}, err
	}

	previousStart := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	previousEndBase := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	previousEnd := previousEndBase.AddDate(0, 1, -1)
	return dateRange{start: previousStart.Format("2006-01-02"), end: previousEnd.Format("2006-01-02")}, nil
}

func (r *Repo) ActiveMembersRenewalRangeCurrent(ctx context.Context, current dateRange, report bool) ([]orderedRow, error) {
	previous, err := previousMonthRange(current)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, month := range []dateRange{current, previous} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+activeMembersRenewalTable+" WHERE start_date = ? AND end_date = ?", month.start, month.end); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, activeMembersRenewalInsertSQL(month)); err != nil {
			return nil, err
		}
	}

	var rows *sql.Rows
	if report {
		rows, err = tx.QueryContext(ctx, "SELECT start_date, SUM(active) AS active, SUM(active_ex) AS active_ex, SUM(`new`) AS `new`, SUM(renew) AS renew, SUM(late_renew) AS late_renew, SUM(due) AS due, SUM(grace) AS grace, SUM(dropout) AS dropout, status FROM "+activeMembersRenewalTable+" WHERE start_date = ? GROUP BY status", current.start)
	} else {
		rows, err = tx.QueryContext(ctx, "SELECT * FROM "+activeMembersRenewalTable+" WHERE start_date BETWEEN ? AND ?", previous.start, current.start)
	}
	if err != nil {
		return nil, err
	}

	result, err := scanRows(rows)
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if !report {
		result = coerceActiveMembersRenewalDetailRows(result)
	}

	return result, nil
}

func (r *Repo) ActiveMembersRenewalRangePast(ctx context.Context, maxEndDate string, page int, pageSize int) (paginatedRows, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+activeMembersRenewalTable+" WHERE end_date <= ?", maxEndDate).Scan(&total); err != nil {
		return paginatedRows{}, err
	}

	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT * FROM "+activeMembersRenewalTable+" WHERE end_date <= ? ORDER BY end_date DESC, start_date DESC, contact_id DESC LIMIT ? OFFSET ?",
		maxEndDate,
		pageSize,
		offset,
	)
	if err != nil {
		return paginatedRows{}, err
	}
	defer rows.Close()

	result, err := scanRows(rows)
	if err != nil {
		return paginatedRows{}, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	return paginatedRows{
		Rows:       coerceActiveMembersRenewalDetailRows(result),
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

var activeMembersRenewalIntColumns = map[string]struct{}{
	"contact_id": {},
	"venue_id":   {},
	"months":     {},
}

var activeMembersRenewalFloatColumns = map[string]struct{}{
	"years": {},
}

var activeMembersRenewalBoolColumns = map[string]struct{}{
	"online":     {},
	"offline":    {},
	"active":     {},
	"active_ex":  {},
	"new":        {},
	"renew":      {},
	"late_renew": {},
	"due":        {},
	"grace":      {},
	"dropout":    {},
	"managed_by_coco": {},
}

func coerceActiveMembersRenewalDetailRows(rows []orderedRow) []orderedRow {
	for rowIndex := range rows {
		for column, value := range rows[rowIndex].values {
			if _, ok := activeMembersRenewalIntColumns[column]; ok {
				rows[rowIndex].values[column] = coerceIntValue(value)
				continue
			}
			if _, ok := activeMembersRenewalFloatColumns[column]; ok {
				rows[rowIndex].values[column] = coerceFloatValue(value)
				continue
			}
			if _, ok := activeMembersRenewalBoolColumns[column]; ok {
				rows[rowIndex].values[column] = coerceBoolValue(value)
			}
		}
	}
	return rows
}

func coerceIntValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if typed == "" {
			return typed
		}
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return value
		}
		return parsed
	default:
		return value
	}
}

func coerceFloatValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		if typed == "" {
			return typed
		}
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return value
		}
		return parsed
	default:
		return value
	}
}

func coerceBoolValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		if typed == "" {
			return typed
		}
		if typed == "1" {
			return true
		}
		if typed == "0" {
			return false
		}
		return value
	default:
		return value
	}
}

func activeMembersRenewalInsertSQL(month dateRange) string {
	return fmt.Sprintf(`
INSERT INTO %s(contact_id, fullname, t_fullname, master_trainer_fullname, dropout_reason, start_date, end_date, type, branch, venue, venue_id, online, offline, dob, years, months, pay_date, member_creation_date, invoice_start_date, invoice_end_date, duration, due_date, grace_end_date, next_payment_date, active, active_ex, ` + "`new`" + `, renew, late_renew, due, grace, dropout, status, first_payment_date, managed_by_coco)
WITH future_members AS (
	SELECT
		i.contact_id,
		CONCAT(c.fname, ' ', c.mname, ' ', c.lname) AS fullname,
		CONCAT(c3.fname, ' ', c3.mname, ' ', c3.lname) AS t_fullname,
		CONCAT(mtc.fname, ' ', mtc.mname, ' ', mtc.lname) AS master_trainer_fullname,
		'' AS dropout_reason,
		'%s' AS start_date,
		'%s' AS end_date,
		b.type,
		b.branch,
		CASE WHEN v.venue IS NULL THEN 'Online' ELSE v.venue END AS venue,
		CASE WHEN v.id IS NULL THEN '0' ELSE v.id END AS venue_id,
		CASE WHEN ct.master_category_id = '1' THEN 1 ELSE 0 END AS online,
		CASE WHEN ct.master_category_id = '2' THEN 1 ELSE 0 END AS offline,
		c.dob AS dob,
		ROUND(TIMESTAMPDIFF(DAY, c.dob, CURDATE()) / 365.25, 1) AS years,
		TIMESTAMPDIFF(MONTH, c.dob, CURDATE()) AS months,
		p.date AS pay_date,
		m.created_at AS member_creation_date,
		ii.start_date AS invoice_start_date,
		ii.end_date AS invoice_end_date,
		CASE
			WHEN s.duration = 1 AND s.duration_type = 'Y' THEN '12M'
			WHEN s.duration = 2 AND s.duration_type = 'Y' THEN '24M'
			ELSE CONCAT(s.duration, s.duration_type)
		END AS duration,
		ii.end_date AS due_date,
		DATE_ADD(ii.end_date, INTERVAL 9 DAY) AS grace_end_date,
		'0000-00-00' AS next_payment_date,
		0 AS active,
		0 AS active_ex,
		1 AS ` + "`new`" + `,
		0 AS renew,
		0 AS late_renew,
		0 AS due,
		0 AS grace,
		0 AS dropout,
		'Member' AS status,
		(
			SELECT MIN(fp.date)
			FROM pf_TickleRight_9210.invoice fi
			JOIN pf_TickleRight_9210.payment fp ON fp.invoice_id = fi.id
			WHERE fi.contact_id = i.contact_id
				AND fi.park = 0
				AND fp.park = 0
		) AS first_payment_date,
		CASE
			WHEN (
				i.bid IN (
					SELECT id
					FROM pf_TickleRight_9210.branch
					WHERE counsellors_incentives = 1
						AND park = 0
				) OR b.type = 'COCO'
			) THEN 1
			ELSE 0
		END AS managed_by_coco
	FROM
		pf_TickleRight_9210.invoice i
		JOIN pf_TickleRight_9210.invoice_item ii ON i.id = ii.invoice_id
		JOIN pf_TickleRight_9210.contact c ON c.id = i.contact_id
		LEFT JOIN pf_TickleRight_9210.member m ON m.contact_id = c.id
		LEFT JOIN pf_TickleRight_9210.branch b ON b.id = i.bid
		LEFT JOIN pf_TickleRight_9210.batch ba ON ba.id = ii.batch_id
		LEFT JOIN pf_TickleRight_9210.venue v ON v.id = ba.venue_id
		LEFT JOIN pf_TickleRight_9210.category ct ON ct.id = ii.category_id
		LEFT JOIN pf_TickleRight_9210.payment p ON p.invoice_id = i.id
		LEFT JOIN pf_TickleRight_9210.contact c3 ON m.t_contact_id = c3.id
		LEFT JOIN pf_TickleRight_9210.employee e_t ON e_t.contact_id = m.t_contact_id
		LEFT JOIN pf_TickleRight_9210.contact mtc ON mtc.id = e_t.master_trainer_contact_id
		LEFT JOIN (
			SELECT i.contact_id AS contact_id, icl.dropout_reason AS dropout_reason
			FROM
				pf_TickleRight_9210.invoice_change_log icl
				JOIN pf_TickleRight_9210.invoice i ON i.id = icl.invoice_id
				JOIN (
					SELECT contact_id, MAX(id) AS latest_invoice_id
					FROM pf_TickleRight_9210.invoice
					WHERE park = 0
					GROUP BY contact_id
				) latest_invoice ON i.id = latest_invoice.latest_invoice_id
			WHERE icl.change = 7
		) idl ON m.contact_id = idl.contact_id
		LEFT JOIN pf_TickleRight_9210.service s ON s.id = ii.service_id
	WHERE
		p.date BETWEEN '%s' AND '%s'
		AND ii.start_date > '%s'
		AND i.park = 0
		AND ii.park = 0
		AND ii.sessions >= 4
		AND c.bid <> 35
		AND b.branch NOT LIKE '%%CCC%%'
		AND b.branch NOT LIKE '%%Testing%%'
		AND b.park = 0
		AND (
			(
				SELECT COUNT(id)
				FROM pf_TickleRight_9210.invoice
				WHERE contact_id = i.contact_id AND id <= i.id AND park = 0
			) = 1
		)
	GROUP BY i.contact_id
),
active_members AS (
	SELECT
		i.contact_id,
		CONCAT(c.fname, ' ', c.mname, ' ', c.lname) AS fullname,
		CONCAT(c3.fname, ' ', c3.mname, ' ', c3.lname) AS t_fullname,
		CONCAT(mtc.fname, ' ', mtc.mname, ' ', mtc.lname) AS master_trainer_fullname,
		CASE
			WHEN (ii.end_date BETWEEN '%s' AND '%s' AND m.status = '2') THEN (
				CASE
					WHEN (
						(
							SELECT payment.date
							FROM pf_TickleRight_9210.invoice
								JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
							WHERE invoice.contact_id = i.contact_id
								AND payment.id > p.id AND payment.invoice_id != i.id
								AND payment.park = 0 AND invoice.park = 0
							ORDER BY payment.id ASC
							LIMIT 1
						) IS NULL
						AND (CURDATE() >= DATE_ADD(ii.end_date, INTERVAL 9 DAY))
					) THEN (
						CASE
							WHEN m.status = '2' THEN (
								CASE
									WHEN m.dropout_comment = 'Other' THEN CONCAT(m.dropout_comment, ' - ', m.dropout_other_reason)
									WHEN idl.dropout_reason != '' AND idl.dropout_reason IS NOT NULL THEN idl.dropout_reason
									ELSE m.dropout_comment
								END
							)
							ELSE ''
						END
					)
					ELSE ''
				END
			)
		END AS dropout_reason,
		'%s' AS start_date,
		'%s' AS end_date,
		b.type,
		b.branch,
		CASE WHEN v.venue IS NULL THEN 'Online' ELSE v.venue END AS venue,
		CASE WHEN v.id IS NULL THEN '0' ELSE v.id END AS venue_id,
		CASE WHEN ct.master_category_id = '1' THEN 1 ELSE 0 END AS online,
		CASE WHEN ct.master_category_id = '2' THEN 1 ELSE 0 END AS offline,
		c.dob AS dob,
		ROUND(TIMESTAMPDIFF(DAY, c.dob, CURDATE()) / 365.25, 1) AS years,
		TIMESTAMPDIFF(MONTH, c.dob, CURDATE()) AS months,
		p.date AS pay_date,
		m.created_at AS member_creation_date,
		ii.start_date AS invoice_start_date,
		ii.end_date AS invoice_end_date,
		CASE
			WHEN s.duration = 1 AND s.duration_type = 'Y' THEN '12M'
			WHEN s.duration = 2 AND s.duration_type = 'Y' THEN '24M'
			ELSE CONCAT(s.duration, s.duration_type)
		END AS duration,
		ii.end_date AS due_date,
		DATE_ADD(ii.end_date, INTERVAL 9 DAY) AS grace_end_date,
		(
			SELECT payment.date
			FROM pf_TickleRight_9210.invoice
				JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
			WHERE invoice.contact_id = i.contact_id
				AND payment.id > p.id AND payment.invoice_id != i.id
				AND payment.park = 0 AND invoice.park = 0
			ORDER BY payment.id ASC
			LIMIT 1
		) AS next_payment_date,
		CASE WHEN ii.end_date > '%s' OR ii.end_date >= CURDATE() THEN 1 ELSE 0 END AS active,
		CASE
			WHEN (
				(
					SELECT invoice_item.end_date
					FROM pf_TickleRight_9210.invoice
						JOIN pf_TickleRight_9210.invoice_item ON invoice.id = invoice_item.invoice_id
					WHERE invoice.contact_id = i.contact_id
						AND invoice.id < i.id
						AND invoice_item.park = 0 AND invoice.park = 0
					ORDER BY invoice.id DESC
					LIMIT 1
				) IS NOT NULL
				AND (
					SELECT invoice_item.end_date
					FROM pf_TickleRight_9210.invoice
						JOIN pf_TickleRight_9210.invoice_item ON invoice.id = invoice_item.invoice_id
					WHERE invoice.contact_id = i.contact_id
						AND invoice.id < i.id
						AND invoice_item.park = 0 AND invoice.park = 0
					ORDER BY invoice.id DESC
					LIMIT 1
				) < DATE_SUB(p.date, INTERVAL 30 DAY)
				AND ii.start_date BETWEEN '%s' AND '%s'
			) THEN 1 ELSE 0
		END AS active_ex,
		CASE
			WHEN (
				(
					(
						SELECT COUNT(id)
						FROM pf_TickleRight_9210.invoice
						WHERE contact_id = i.contact_id AND id <= i.id AND park = 0
					) = 1
				)
				AND '%s' <= p.date
				AND ii.start_date BETWEEN '%s' AND '%s'
			) THEN 1
			ELSE 0
		END AS ` + "`new`" + `,
		CASE
			WHEN (
				ii.end_date BETWEEN '%s' AND '%s'
				AND (
					SELECT payment.date
					FROM pf_TickleRight_9210.invoice
						JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
					WHERE invoice.contact_id = i.contact_id
						AND payment.id > p.id AND payment.invoice_id != i.id
						AND payment.park = 0 AND invoice.park = 0
					ORDER BY payment.id ASC
					LIMIT 1
				) IS NOT NULL
			) THEN 1 ELSE 0
		END AS renew,
		CASE
			WHEN ii.end_date BETWEEN '%s' AND '%s' THEN (
				CASE
					WHEN (
						SELECT payment.date
						FROM pf_TickleRight_9210.invoice
							JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
						WHERE invoice.contact_id = i.contact_id
							AND payment.id > p.id AND payment.invoice_id != i.id
							AND payment.park = 0 AND invoice.park = 0
						ORDER BY payment.id ASC
						LIMIT 1
					) IS NOT NULL THEN (
						CASE
							WHEN (
								SELECT payment.date
								FROM pf_TickleRight_9210.invoice
									JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
								WHERE invoice.contact_id = i.contact_id
									AND payment.id > p.id AND payment.invoice_id != i.id
									AND payment.park = 0 AND invoice.park = 0
								ORDER BY payment.id ASC
								LIMIT 1
							) > DATE_ADD(ii.end_date, INTERVAL 9 DAY) THEN 1
							ELSE 0
						END
					)
					ELSE 0
				END
			)
		END AS late_renew,
		CASE WHEN ii.end_date BETWEEN '%s' AND '%s' THEN 1 ELSE 0 END AS due,
		CASE
			WHEN (
				ii.end_date BETWEEN '%s' AND '%s'
				AND (
					SELECT payment.date
					FROM pf_TickleRight_9210.invoice
						JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
					WHERE invoice.contact_id = i.contact_id
						AND payment.id > p.id AND payment.invoice_id != i.id
						AND payment.park = 0 AND invoice.park = 0
					ORDER BY payment.id ASC
					LIMIT 1
				) IS NULL
				AND CURDATE() <= DATE_ADD(ii.end_date, INTERVAL 9 DAY)
				AND CURDATE() >= ii.end_date
			) THEN 1 ELSE 0
		END AS grace,
		CASE
			WHEN ii.end_date < CURDATE() THEN (
				CASE
					WHEN (
						SELECT payment.date
						FROM pf_TickleRight_9210.invoice
							JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
						WHERE invoice.contact_id = i.contact_id
							AND payment.id > p.id
							AND payment.invoice_id != i.id
							AND payment.park = 0
							AND invoice.park = 0
						ORDER BY payment.id ASC
						LIMIT 1
					) IS NULL THEN 1
					ELSE 0
				END
			) ELSE 0
		END AS dropout,
		CASE
			WHEN ii.end_date BETWEEN '%s' AND '%s' THEN (
				CASE
					WHEN (
						SELECT payment.date
						FROM pf_TickleRight_9210.invoice
							JOIN pf_TickleRight_9210.payment ON invoice.id = payment.invoice_id
						WHERE invoice.contact_id = i.contact_id
							AND payment.id > p.id AND payment.invoice_id != i.id
							AND payment.park = 0 AND invoice.park = 0
						ORDER BY payment.id ASC
						LIMIT 1
					) IS NOT NULL THEN 'Renew'
					WHEN ii.end_date >= CURDATE() THEN 'Active' ELSE 'Dropout'
				END
			)
			WHEN ii.end_date >= '%s' THEN (
				CASE
					WHEN (
						(
							SELECT invoice_item.end_date
							FROM pf_TickleRight_9210.invoice
								JOIN pf_TickleRight_9210.invoice_item ON invoice.id = invoice_item.invoice_id
							WHERE invoice.contact_id = i.contact_id
								AND invoice.id < i.id
								AND invoice_item.park = 0 AND invoice.park = 0
							ORDER BY invoice.id DESC
							LIMIT 1
						) IS NOT NULL
						AND (
							SELECT invoice_item.end_date
							FROM pf_TickleRight_9210.invoice
								JOIN pf_TickleRight_9210.invoice_item ON invoice.id = invoice_item.invoice_id
							WHERE invoice.contact_id = i.contact_id
								AND invoice.id < i.id
								AND invoice_item.park = 0 AND invoice.park = 0
							ORDER BY invoice.id DESC
							LIMIT 1
						) < DATE_SUB(p.date, INTERVAL 30 DAY)
						AND ii.start_date BETWEEN '%s' AND '%s'
					) THEN 'Active-Ex' ELSE 'Active'
				END
			)
			WHEN ii.end_date < '%s' THEN 'Dropout'
		END AS status,
		(
			SELECT MIN(fp.date)
			FROM pf_TickleRight_9210.invoice fi
			JOIN pf_TickleRight_9210.payment fp ON fp.invoice_id = fi.id
			WHERE fi.contact_id = i.contact_id
				AND fi.park = 0
				AND fp.park = 0
		) AS first_payment_date,
		CASE
			WHEN (
				i.bid IN (
					SELECT id
					FROM pf_TickleRight_9210.branch
					WHERE counsellors_incentives = 1
						AND park = 0
				) OR b.type = 'COCO'
			) THEN 1
			ELSE 0
		END AS managed_by_coco
	FROM
		pf_TickleRight_9210.invoice i
		JOIN pf_TickleRight_9210.invoice_item ii ON i.id = ii.invoice_id
		JOIN pf_TickleRight_9210.contact c ON c.id = i.contact_id
		LEFT JOIN pf_TickleRight_9210.member m ON m.contact_id = c.id
		LEFT JOIN pf_TickleRight_9210.branch b ON b.id = i.bid
		LEFT JOIN pf_TickleRight_9210.batch ba ON ba.id = ii.batch_id
		LEFT JOIN pf_TickleRight_9210.venue v ON v.id = ba.venue_id
		LEFT JOIN pf_TickleRight_9210.category ct ON ct.id = ii.category_id
		LEFT JOIN pf_TickleRight_9210.payment p ON p.invoice_id = i.id
		LEFT JOIN pf_TickleRight_9210.contact c3 ON m.t_contact_id = c3.id
		LEFT JOIN pf_TickleRight_9210.employee e_t ON e_t.contact_id = m.t_contact_id
		LEFT JOIN pf_TickleRight_9210.contact mtc ON mtc.id = e_t.master_trainer_contact_id
		LEFT JOIN (
			SELECT i.contact_id AS contact_id, icl.dropout_reason AS dropout_reason
			FROM
				pf_TickleRight_9210.invoice_change_log icl
				JOIN pf_TickleRight_9210.invoice i ON i.id = icl.invoice_id
				JOIN (
					SELECT contact_id, MAX(id) AS latest_invoice_id
					FROM pf_TickleRight_9210.invoice
					WHERE park = 0
					GROUP BY contact_id
				) latest_invoice ON i.id = latest_invoice.latest_invoice_id
			WHERE icl.change = 7
		) idl ON m.contact_id = idl.contact_id
		LEFT JOIN pf_TickleRight_9210.service s ON s.id = ii.service_id
	WHERE
		'%s' >= ii.start_date
		AND '%s' <= ii.end_date
		AND i.park = 0
		AND ii.park = 0
		AND ii.sessions >= 4
		AND c.bid <> 35
		AND b.branch NOT LIKE '%%CCC%%'
		AND b.branch NOT LIKE '%%Testing%%'
		AND b.park = 0
	GROUP BY i.contact_id
)
SELECT * FROM future_members
UNION ALL
SELECT * FROM active_members`,
		activeMembersRenewalTable,
		month.start, month.end,
		month.start, month.end, month.end,
		month.start, month.end,
		month.start, month.end,
		month.end,
		month.start, month.end,
		month.start, month.start, month.end,
		month.start, month.end,
		month.start, month.end,
		month.start, month.end,
		month.start, month.end,
		month.start, month.end,
		month.end,
		month.start, month.end,
		month.end,
		month.end, month.start)
}
