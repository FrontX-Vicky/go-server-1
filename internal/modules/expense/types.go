package expense

// ExpenseListRequest carries pagination, date range, and optional search filters.
type ExpenseListRequest struct {
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Search       string `json:"search"`
	Limit        int    `json:"limit"`
	Offset       int    `json:"offset"`
	QueryOnly    bool   `json:"query_only"`
}

// ExpenseRow mirrors the columns exposed by expense_view.
type ExpenseRow struct {
	ID                           any    `json:"id"`
	EcatID                       any    `json:"ecat_id"`
	Category                     any    `json:"category"`
	PaymentDone                  any    `json:"payment_done"`
	PaymentReject                any    `json:"payment_reject"`
	ExpenseRejectComment         any    `json:"expense_reject_comment"`
	Employee                     any    `json:"employee"`
	Reason                       any    `json:"reason"`
	Amount                       any    `json:"amount"`
	TransactionID                any    `json:"transaction_id"`
	DistributeIn                 any    `json:"distribute_in"`
	DistributeInStr              any    `json:"distribute_in_str"`
	PayMode                      any    `json:"pay_mode"`
	PayModeString                any    `json:"pay_mode_string"`
	Date                         any    `json:"date"`
	DateString                   any    `json:"date_string"`
	EmployeeID                   any    `json:"employee_id"`
	EmpName                      any    `json:"emp_name"`
	ToEmployeeID                 any    `json:"to_employee_id"`
	ToEmployeeName               any    `json:"to_employee_name"`
	ExpenseBalance               any    `json:"expense_balance"`
	ResultingExpenseBalance      any    `json:"resulting_expense_balance"`
	Attachments                  any    `json:"attachments"`
	Attachments1                 any    `json:"attachments1"`
	Attachments2                 any    `json:"attachments2"`
	NoOfAttachments              any    `json:"no_of_attachments"`
	Comment                      any    `json:"comment"`
	Park                         any    `json:"park"`
	Bid                          any    `json:"bid"`
	Payment                      any    `json:"payment"`
	Branch                       any    `json:"branch"`
	Venue                        any    `json:"venue"`
	VenueID                      any    `json:"venue_id"`
	RelationID                   any    `json:"relation_id"`
	Cleared                      any    `json:"cleared"`
	DetailedData                 any    `json:"detailedData"`
	LedgerData                   any    `json:"ledger_data"`
	ApprovedExpense              any    `json:"approved_expense"`
	ApprovedDate                 any    `json:"approved_date"`
	TypeHead                     any    `json:"type_head"`
	TypeHead1                    any    `json:"type_head1"`
	TypeHead2                    any    `json:"type_head2"`
	TypeHead3                    any    `json:"type_head3"`
	TypeHeadName                 any    `json:"type_head_name"`
	TypeHead1Name                any    `json:"type_head1_name"`
	TypeHead2Name                any    `json:"type_head2_name"`
	TypeHead3Name                any    `json:"type_head3_name"`
	TypeHeadCode                 any    `json:"type_head_code"`
	TypeHead1Code                any    `json:"type_head1_code"`
	TypeHead2Code                any    `json:"type_head2_code"`
	TypeHead3Code                any    `json:"type_head3_code"`
	Tax1                         any    `json:"tax_1"`
	Tax2                         any    `json:"tax_2"`
	Tax3                         any    `json:"tax_3"`
	TotalAmount                  any    `json:"total_amount"`
	Tr1                          any    `json:"tr_1"`
	Tr2                          any    `json:"tr_2"`
	GlobalExpensePending         any    `json:"global_expense_pending"`
	GlobalExpenseApproved        any    `json:"global_expense_approved"`
	GlobalExpensePaymentApproved any    `json:"global_expense_payment_approved"`
	CreatedBy                    any    `json:"created_by"`
	CreatedAt                    any    `json:"created_at"`
	ModifiedBy                   any    `json:"modified_by"`
	ModifiedAt                   any    `json:"modified_at"`
}

// ExpensePagination holds paging metadata returned to the client.
type ExpensePagination struct {
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	TotalCount int `json:"total_count"`
	PageCount  int `json:"page_count"`
}

// ExpenseListResponse is the full response for the expense list endpoint.
type ExpenseListResponse struct {
	Rows       []map[string]any  `json:"rows"`
	Pagination ExpensePagination `json:"pagination"`
}

// ParticularOption represents an option for the Particulars dropdown.
type ParticularOption struct {
	ID       int    `json:"id"`
	Category string `json:"category"`
}

// TypeHeadOption represents a mapped option for the Level 3 Detail combobox.
type TypeHeadOption struct {
	ID            int    `json:"id"`
	ReferenceCode int    `json:"reference_code"`
	TypeOfExpense string `json:"type_of_expense"`
	TypeHead1     int    `json:"type_head1"`
	TypeHead2     int    `json:"type_head2"`
	TypeHead3     int    `json:"type_head3"`
}

// ExpenseOptions holds the dropdown options for expense inline editing.
type ExpenseOptions struct {
	Particulars []ParticularOption `json:"particulars"`
	TypeHeads   []TypeHeadOption   `json:"type_heads"`
}

// UpdateExpenseInlineRequest represents the payload for inline editing an expense.
type UpdateExpenseInlineRequest struct {
	EcatID       int `json:"ecat_id"`
	DistributeIn int `json:"distribute_in"`
	TypeHead     int `json:"type_head"`
	TypeHead1    int `json:"type_head1"`
	TypeHead2    int `json:"type_head2"`
	TypeHead3    int `json:"type_head3"`
}

