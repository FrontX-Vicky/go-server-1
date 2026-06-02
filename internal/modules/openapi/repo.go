package openapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"server_1/internal/core/db"
)

type Repo struct {
	db *sql.DB
}

func NewRepo() *Repo {
	return &Repo{db: db.DB("DB1")}
}

const inquiryDemoFollowupSQL = `
SELECT
						i.contact_id AS contact_id,
						CONCAT(c.fname, ' ', c.lname, ' ', c.mname) AS fullname,
						c.dob,
						f.city AS city,
						v.venue,
						f.country AS country,
						CONCAT(ce.fname, ' ', ce.lname) AS counsellor_name,
						b.branch,
						b.type AS branch_type,
						CASE
							WHEN cat.master_category_id = 1 THEN 'Online Program'
							WHEN cat.master_category_id = 2 THEN 'Offline Program'
							ELSE ''
						END AS program_type,
						i.created_at AS doi_created,
						i.allocation_date,
						f.interest_string AS interest_string,
						d.start_date AS demo_date,
						dfr.demo_attended,
						COALESCE(fp.first_payment_date, NULL) AS date_of_conversion,
						CASE
							WHEN m.id IS NULL THEN 0
							ELSE 1
						END AS inquiry_converted,
						f.lost_reason AS lost_reason,
						f.comment AS COMMENT,
						s.source,
						ps.source AS primary_source,
						cp.campaign_name,
						h.heard_from AS heard_from,
						ph.heard_from AS primary_heard_from,
						i.utm AS utm,
						CASE
							WHEN f.interest_string IN ('Hot', 'Warm') THEN 'Yes'
							WHEN (
								CASE
									WHEN m.id IS NULL THEN 0
									ELSE 1
								END
							) = 1 THEN 'Yes'
							WHEN f.interest_string = 'Cold'
							AND f.lost_reason IN (
								'Expensive',
								'No Device for Online Program',
								'Requested for a Trial Class',
								'Location Proximity',
								'No Centre in the City',
								'Batch Unavailability/Batches Maxed Out'
							) THEN 'Yes'
							ELSE 'No'
							END AS ` + "`sql`" + `,
							COALESCE(fp.first_payment_amount, 0) AS first_payment_amount,
							COALESCE(fp.first_payment_date, NULL) AS first_payment_date,
							fi.invoice_id AS id,
							fd.duration,
							COALESCE(fp.pay_mode, NULL) AS pay_mode,
							1 AS managed_by_coco
					FROM
						pf_TickleRight_9210.inquiry i
						LEFT JOIN pf_TickleRight_9210.contact c ON i.contact_id = c.id
						AND i.park = 0
						AND c.park = 0
						LEFT JOIN (
							/* latest followup per contact (by id) */
							SELECT
								f1.*
							FROM
								pf_TickleRight_9210.followup f1
								JOIN (
									SELECT
										contact_id,
										MAX(id) AS max_id
									FROM
										pf_TickleRight_9210.followup
									WHERE
										park = 0
										AND master_id <> 0
									GROUP BY
										contact_id
								) f2 ON f2.max_id = f1.id
						) f ON f.contact_id = i.contact_id
						LEFT JOIN pf_TickleRight_9210.venue v ON v.id = f.venue_id
						LEFT JOIN pf_TickleRight_9210.employee e ON e.id = i.c_employee_id
						LEFT JOIN pf_TickleRight_9210.contact ce ON ce.id = e.contact_id
						LEFT JOIN pf_TickleRight_9210.branch b ON b.id = i.bid
						LEFT JOIN (
							/* latest demo per mobile */
							SELECT
								mobile,
								MAX(demo_id) AS demo_id,
								MAX(demo_attended) AS demo_attended
							FROM
								pf_TickleRight_9210.demo_form_response
							GROUP BY
								mobile
						) dfr ON dfr.mobile = c.mobile
						LEFT JOIN pf_TickleRight_9210.demo d ON d.id = dfr.demo_id
						LEFT JOIN pf_TickleRight_9210.member m ON m.contact_id = i.contact_id
						LEFT JOIN pf_TickleRight_9210.inquiry_source s ON s.id = i.source
						LEFT JOIN pf_TickleRight_9210.inquiry_source ps ON ps.id = i.primary_source
						LEFT JOIN pf_TickleRight_9210.heard_from h ON h.id = i.heard_from
						LEFT JOIN pf_TickleRight_9210.heard_from ph ON ph.id = i.primary_heard_from
						LEFT JOIN pf_TickleRight_9210.campaign cp ON cp.campaign_id = h.campaign_id
						LEFT JOIN pf_TickleRight_9210.inquiry_category ic ON ic.inquiry_id = i.id
						LEFT JOIN pf_TickleRight_9210.category cat ON cat.id = ic.category_id
						/* --------- earliest non-parked invoice per contact (by date, then id) ---------- */
						LEFT JOIN (
							SELECT
								i1.contact_id,
								i1.id AS invoice_id
							FROM
								pf_TickleRight_9210.invoice i1
								LEFT JOIN pf_TickleRight_9210.invoice i2 ON i2.contact_id = i1.contact_id
								AND i2.park = 0
								AND (
									i2.date < i1.date
									OR (
										i2.date = i1.date
										AND i2.id < i1.id
									)
								)
							WHERE
								i1.park = 0
								AND i2.id IS NULL
						) fi ON fi.contact_id = i.contact_id
							/* first non-parked payment for that invoice (by date, then id) */
							LEFT JOIN (
								SELECT
									p1.invoice_id,
									CASE
										WHEN p1.pay_mode = 1 THEN p1.amount
										ELSE p1.calculated_amount
									END AS first_payment_amount,
									p1.date AS first_payment_date,
									p1.pay_mode AS pay_mode
								FROM
									pf_TickleRight_9210.payment p1
									LEFT JOIN pf_TickleRight_9210.payment p2 ON p2.invoice_id = p1.invoice_id
									AND p2.park = 0
									AND (
										p2.date < p1.date
										OR (
											p2.date = p1.date
											AND p2.id < p1.id
										)
									)
								WHERE
									p1.park = 0
									AND p2.id IS NULL
							) fp ON fp.invoice_id = fi.invoice_id
						/* duration from a single, deterministic invoice item (the earliest item per invoice) */
						LEFT JOIN (
							SELECT
								ii.invoice_id,
								CASE
									WHEN s.duration = 1
									AND s.duration_type = 'Y' THEN '12M'
									WHEN s.duration = 2
									AND s.duration_type = 'Y' THEN '24M'
									ELSE CONCAT(s.duration, s.duration_type)
								END AS duration
							FROM
								pf_TickleRight_9210.invoice_item ii
								JOIN (
									SELECT
										invoice_id,
										MIN(id) AS min_item_id
									FROM
										pf_TickleRight_9210.invoice_item
									GROUP BY
										invoice_id
								) pick ON pick.invoice_id = ii.invoice_id
								AND pick.min_item_id = ii.id
								JOIN pf_TickleRight_9210.service s ON s.id = ii.service_id
						) fd ON fd.invoice_id = fi.invoice_id
					WHERE
						i.created_at >= '2024-01-01'
						AND i.park = 0
						AND c.park = 0
						AND f.followup IN (0, 2)
						AND (i.bid IN(
    SELECT
        id
    FROM
        pf_TickleRight_9210.branch
    WHERE
        counsellors_incentives = 1 AND park = 0
) OR b.type = 'COCO')

UNION ALL

SELECT
						i.contact_id AS contact_id,
						CONCAT(c.fname, ' ', c.lname, ' ', c.mname) AS fullname,
						c.dob,
						f.city AS city,
						v.venue,
						f.country AS country,
						CONCAT(ce.fname, ' ', ce.lname) AS counsellor_name,
						b.branch,
						b.type AS branch_type,
						CASE
							WHEN cat.master_category_id = 1 THEN 'Online Program'
							WHEN cat.master_category_id = 2 THEN 'Offline Program'
							ELSE ''
						END AS program_type,
						i.created_at AS doi_created,
						i.allocation_date,
						f.interest_string AS interest_string,
						d.start_date AS demo_date,
						dfr.demo_attended,
						COALESCE(fp.first_payment_date, NULL) AS date_of_conversion,
						CASE
							WHEN m.id IS NULL THEN 0
							ELSE 1
						END AS inquiry_converted,
						f.lost_reason AS lost_reason,
						f.comment AS COMMENT,
						s.source,
						ps.source AS primary_source,
						cp.campaign_name,
						h.heard_from AS heard_from,
						ph.heard_from AS primary_heard_from,
						i.utm AS utm,
						CASE
							WHEN f.interest_string IN ('Hot', 'Warm') THEN 'Yes'
							WHEN (
								CASE
									WHEN m.id IS NULL THEN 0
									ELSE 1
								END
							) = 1 THEN 'Yes'
							WHEN f.interest_string = 'Cold'
							AND f.lost_reason IN (
								'Expensive',
								'No Device for Online Program',
								'Requested for a Trial Class',
								'Location Proximity',
								'No Centre in the City',
								'Batch Unavailability/Batches Maxed Out'
							) THEN 'Yes'
							ELSE 'No'
							END AS ` + "`sql`" + `,
							COALESCE(fp.first_payment_amount, 0) AS first_payment_amount,
							COALESCE(fp.first_payment_date, NULL) AS first_payment_date,
							fi.invoice_id AS id,
							fd.duration,
							COALESCE(fp.pay_mode, NULL) AS pay_mode,
							0 AS managed_by_coco
					FROM
						pf_TickleRight_9210.inquiry i
						LEFT JOIN pf_TickleRight_9210.contact c ON i.contact_id = c.id
						AND i.park = 0
						AND c.park = 0
						LEFT JOIN (
							/* latest followup per contact (by id) */
							SELECT
								f1.*
							FROM
								pf_TickleRight_9210.followup f1
								JOIN (
									SELECT
										contact_id,
										MAX(id) AS max_id
									FROM
										pf_TickleRight_9210.followup
									WHERE
										park = 0
										AND master_id <> 0
									GROUP BY
										contact_id
								) f2 ON f2.max_id = f1.id
						) f ON f.contact_id = i.contact_id
						LEFT JOIN pf_TickleRight_9210.venue v ON v.id = f.venue_id
						LEFT JOIN pf_TickleRight_9210.employee e ON e.id = i.c_employee_id
						LEFT JOIN pf_TickleRight_9210.contact ce ON ce.id = e.contact_id
						LEFT JOIN pf_TickleRight_9210.branch b ON b.id = i.bid
						LEFT JOIN (
							/* latest demo per mobile */
							SELECT
								mobile,
								MAX(demo_id) AS demo_id,
								MAX(demo_attended) AS demo_attended
							FROM
								pf_TickleRight_9210.demo_form_response
							GROUP BY
								mobile
						) dfr ON dfr.mobile = c.mobile
						LEFT JOIN pf_TickleRight_9210.demo d ON d.id = dfr.demo_id
						LEFT JOIN pf_TickleRight_9210.member m ON m.contact_id = i.contact_id
						LEFT JOIN pf_TickleRight_9210.inquiry_source s ON s.id = i.source
						LEFT JOIN pf_TickleRight_9210.inquiry_source ps ON ps.id = i.primary_source
						LEFT JOIN pf_TickleRight_9210.heard_from h ON h.id = i.heard_from
						LEFT JOIN pf_TickleRight_9210.heard_from ph ON ph.id = i.primary_heard_from
						LEFT JOIN pf_TickleRight_9210.campaign cp ON cp.campaign_id = h.campaign_id
						LEFT JOIN pf_TickleRight_9210.inquiry_category ic ON ic.inquiry_id = i.id
						LEFT JOIN pf_TickleRight_9210.category cat ON cat.id = ic.category_id
						/* --------- earliest non-parked invoice per contact (by date, then id) ---------- */
						LEFT JOIN (
							SELECT
								i1.contact_id,
								i1.id AS invoice_id
							FROM
								pf_TickleRight_9210.invoice i1
								LEFT JOIN pf_TickleRight_9210.invoice i2 ON i2.contact_id = i1.contact_id
								AND i2.park = 0
								AND (
									i2.date < i1.date
									OR (
										i2.date = i1.date
										AND i2.id < i1.id
									)
								)
							WHERE
								i1.park = 0
								AND i2.id IS NULL
						) fi ON fi.contact_id = i.contact_id
							/* first non-parked payment for that invoice (by date, then id) */
							LEFT JOIN (
								SELECT
									p1.invoice_id,
									CASE
										WHEN p1.pay_mode = 1 THEN p1.amount
										ELSE p1.calculated_amount
									END AS first_payment_amount,
									p1.date AS first_payment_date,
									p1.pay_mode AS pay_mode
								FROM
									pf_TickleRight_9210.payment p1
									LEFT JOIN pf_TickleRight_9210.payment p2 ON p2.invoice_id = p1.invoice_id
									AND p2.park = 0
									AND (
										p2.date < p1.date
										OR (
											p2.date = p1.date
											AND p2.id < p1.id
										)
									)
								WHERE
									p1.park = 0
									AND p2.id IS NULL
							) fp ON fp.invoice_id = fi.invoice_id
						/* duration from a single, deterministic invoice item (the earliest item per invoice) */
						LEFT JOIN (
							SELECT
								ii.invoice_id,
								CASE
									WHEN s.duration = 1
									AND s.duration_type = 'Y' THEN '12M'
									WHEN s.duration = 2
									AND s.duration_type = 'Y' THEN '24M'
									ELSE CONCAT(s.duration, s.duration_type)
								END AS duration
							FROM
								pf_TickleRight_9210.invoice_item ii
								JOIN (
									SELECT
										invoice_id,
										MIN(id) AS min_item_id
									FROM
										pf_TickleRight_9210.invoice_item
									GROUP BY
										invoice_id
								) pick ON pick.invoice_id = ii.invoice_id
								AND pick.min_item_id = ii.id
								JOIN pf_TickleRight_9210.service s ON s.id = ii.service_id
						) fd ON fd.invoice_id = fi.invoice_id
					WHERE
						i.created_at >= '2024-01-01'
						AND i.park = 0
						AND c.park = 0
						AND f.followup IN (0, 2)
						AND NOT (
    i.bid IN(
        SELECT
            id
        FROM
            pf_TickleRight_9210.branch
        WHERE
            counsellors_incentives = 1 AND park = 0
    ) OR b.type = 'COCO'
)`

func (r *Repo) InquiryDemoFollowup(ctx context.Context) ([]orderedRow, error) {
	rows, err := r.db.QueryContext(ctx, inquiryDemoFollowupSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRows(rows)
}

type orderedRow struct {
	columns []string
	values  map[string]any
}

func (r orderedRow) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	for i, column := range r.columns {
		if i > 0 {
			buf.WriteByte(',')
		}

		key, err := json.Marshal(column)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(r.values[column])
		if err != nil {
			return nil, err
		}

		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func scanRows(rows *sql.Rows) ([]orderedRow, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	result := make([]orderedRow, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeSQLValue(values[i], columnTypes[i].DatabaseTypeName())
		}
		result = append(result, orderedRow{columns: columns, values: row})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func normalizeSQLValue(value any, databaseType string) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		if typed.IsZero() {
			return nil
		}
		if strings.EqualFold(databaseType, "DATE") {
			return typed.Format("2006-01-02")
		}
		return typed.Format("2006-01-02 15:04:05")
	default:
		return typed
	}
}
