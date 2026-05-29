package openapi

import (
	"context"
	"time"
)

const revenueReportSQL = `
SELECT x.* FROM (
	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		_.calculated_amount AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END AS percentage,
		CAST((((_.calculated_amount * (CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100)) AS DECIMAL(10, 2)) AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type = 'Franchisee')
		AND (_.invoice_park = '0')
		AND (_.category = 'Offline Right Brain Development')
		AND (_.clear = '1')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)

	UNION ALL

	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		_.calculated_amount AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END AS percentage,
		CAST((((_.calculated_amount * (CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100)) AS DECIMAL(10, 2)) AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type = 'Franchisee')
		AND (_.invoice_park = '0')
		AND (_.category = 'Right Brain Education')
		AND (_.clear = '1')
		AND (_.park = '0')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)
) x

UNION ALL

SELECT x.* FROM (
	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		CASE
			WHEN (_.bid = 9) THEN (_.calculated_amount)
			WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
			ELSE (_.pay_amount)
		END - IFNULL(p.amount, 0) AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END AS percentage,
		CAST((
			((((CASE
				WHEN (_.bid = 9) THEN (_.calculated_amount)
				WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
				ELSE (_.pay_amount)
			END - IFNULL(p.amount, 0)) * (CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100)
			+
			((((((CASE
				WHEN (_.bid = 9) THEN (_.calculated_amount)
				WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
				ELSE (_.pay_amount)
			END - IFNULL(p.amount, 0)) * (CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100) * 18) / 100)
		) AS DECIMAL(10, 2)) AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type = 'OM_Franchisee')
		AND (_.invoice_park = '0')
		AND (_.category = 'Offline Right Brain Development')
		AND (_.park = '0')
		AND (_.clear = '1')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)

	UNION ALL

	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		((((CASE
			WHEN (_.bid = 9) THEN (_.calculated_amount)
			WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
			ELSE (_.pay_amount)
		END - IFNULL(p.amount, 0))))) AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END AS percentage,
		CAST((
			((((CASE
				WHEN (_.bid = 9) THEN (_.calculated_amount)
				WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
				ELSE (_.pay_amount)
			END - IFNULL(p.amount, 0)) * (CASE WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100)
			+
			(((((CASE
				WHEN (_.bid = 9) THEN (_.calculated_amount)
				WHEN ((_.tax > 0 OR _.item_disallowed_discount > 0 OR _.item_discount > 0) AND _.pay_mode_text != 'Coupon') THEN ((_.amount + _.item_disallowed_discount) - _.item_discount)
				ELSE (_.pay_amount)
			END - IFNULL(p.amount, 0)) * (CASE WHEN (_.bid = 9) THEN (_.calculated_amount) WHEN (_.master_category_id = 2) THEN (_.venue_offline_royalty) ELSE (_.royalty) END)) / 100) * 18) / 100)
		) AS DECIMAL(10, 2)) + _.zoom AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type = 'OM_Franchisee')
		AND (_.invoice_park = '0')
		AND (_.category = 'Right Brain Education')
		AND (_.park = '0')
		AND (_.clear = '1')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)
) x

UNION ALL

SELECT x.* FROM (
	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		_.calculated_amount AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		100 AS percentage,
		_.calculated_amount AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type IN ('COCO', 'TEMP'))
		AND (_.bid NOT IN ('35'))
		AND (_.invoice_park = '0')
		AND (_.category = 'Offline Right Brain Development')
		AND (_.clear = '1')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)

	UNION ALL

	SELECT
		_.contact_id AS id,
		_.code_fullname AS fullname,
		_.amount AS invoice_amount,
		_.category AS category,
		_.branch AS branch,
		_.branch_type AS branch_type,
		_.venue AS venue,
		_.venue_zone AS venue_zone,
		_.pay_date AS pay_date,
		_.invoice_date AS invoice_date,
		_.pay_mode_text AS pay_mode_text,
		_.duration AS package_duration,
		TIMESTAMPDIFF(MONTH, _.start_date, _.end_date) AS actual_duration,
		_.invoice_count AS invoice_count,
		_.pay_amount AS pay_amount,
		_.calculated_amount AS actual_amount,
		DATE_FORMAT(_.date, '%b %y') AS month,
		100 AS percentage,
		CAST(((_.calculated_amount * (100)) / 100) AS DECIMAL(10, 2)) AS final_revenue_amount
	FROM pf_TickleRight_9210.invoice_payment_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment
		WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE (
		(_.date >= '2024-01-01')
		AND (_.pay_mode_text NOT IN ('Points', 'Referral'))
		AND (_.service NOT IN ('Demo Fee'))
		AND (_.branch_type IN ('COCO', 'TEMP'))
		AND (_.bid NOT IN ('35'))
		AND (_.invoice_park = '0')
		AND (_.category = 'Right Brain Education')
		AND (_.clear = '1')
		AND (_.park = '0')
		AND (_.master_category_id IN (1, 2))
		AND _.bid NOT IN (35)
	)
) x
`

const demoFormResponseSQL = `
SELECT
	dfrv.contact_id,
	dfrv.inquiry_id,
	dfrv.c_emp_name,
	dfrv.converted,
	dfrv.source,
	dfrv.heard_from,
	dfrv.lost_reason,
	CASE dfrv.demo_type
		WHEN 0 THEN 'Demo'
		WHEN 1 THEN 'Story Telling'
		WHEN 2 THEN 'Paid Demo'
		WHEN 3 THEN 'Workshop'
		WHEN 4 THEN 'Online Demo'
		ELSE 'Unknown'
	END AS demo_type,
	dfrv.demo_attended,
	dfrv.gender,
	dfrv.name,
	dfrv.dob,
	dfrv.demo_owner_names,
	dfrv.age,
	dfrv.branch,
	dfrv.branch_type,
	dfrv.venue,
	dfrv.demo_date,
	dfrv.created_at,
	s.service,
	p.amount,
	p.date AS pay_date,
	STR_TO_DATE(CONCAT(demo_date, ' ', start_time), '%Y-%m-%d %H:%i:%s') AS demo_date_time,
	fi.invoice_id,
	CASE
		WHEN s.duration = 1 AND s.duration_type = 'Y' THEN '12M'
		WHEN s.duration = 2 AND s.duration_type = 'Y' THEN '24M'
		ELSE CONCAT(s.duration, s.duration_type)
	END AS duration
FROM demo_form_response_view dfrv
LEFT JOIN (
	SELECT
		i1.contact_id,
		i1.id AS invoice_id
	FROM pf_TickleRight_9210.invoice i1
	JOIN (
		SELECT
			contact_id,
			MIN(id) AS first_id
		FROM pf_TickleRight_9210.invoice
		GROUP BY contact_id
	) x ON x.contact_id = i1.contact_id AND x.first_id = i1.id
) fi ON fi.contact_id = dfrv.contact_id
LEFT JOIN invoice_item ii ON ii.invoice_id = fi.invoice_id
LEFT JOIN service s ON ii.service_id = s.id
LEFT JOIN payment p ON fi.invoice_id = p.invoice_id AND p.pay_mode_text != 'Points'
WHERE dfrv.created_at >= '2024-01-01'
ORDER BY dfrv.id ASC
`

func currentMonthToTodayRange(now time.Time) dateRange {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return dateRange{
		start: start.Format("2006-01-02"),
		end:   now.Format("2006-01-02"),
	}
}

func oldMarketingRange(now time.Time) dateRange {
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, now.Location())
	lastMonthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1)
	return dateRange{
		start: start.Format("2006-01-02"),
		end:   lastMonthEnd.Format("2006-01-02"),
	}
}

func (r *Repo) GetCurrentData(ctx context.Context, window dateRange) ([]orderedRow, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM pf_TickleRight_9210.marketing_data WHERE `date` BETWEEN ? AND ?", window.start, window.end)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func (r *Repo) GetOldData(ctx context.Context, window dateRange) ([]orderedRow, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM pf_TickleRight_9210.marketing_data WHERE `date` BETWEEN ? AND ?", window.start, window.end)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func (r *Repo) RevenueReport(ctx context.Context) ([]orderedRow, error) {
	rows, err := r.db.QueryContext(ctx, revenueReportSQL)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func (r *Repo) GetDemoFormResponseData(ctx context.Context) ([]orderedRow, error) {
	rows, err := r.db.QueryContext(ctx, demoFormResponseSQL)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}
