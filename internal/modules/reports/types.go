package reports

// Types used by the reports module.
// These mirror the columns returned by the original SQL query.

type ReportColumn struct {
    ColumnName string `json:"column_name"`
    Header     string `json:"header"`
    IsAdmin    bool   `json:"isAdmin"`
    Position   int    `json:"position"`
    Mpos       int    `json:"mpos"`
    URL        string `json:"url"`
    ConcatID   string `json:"concat_id"`
    OnClick    string `json:"onclick"`
    Modal      string `json:"modal"`
}

type ReportMeta struct {
    ID            int64          `json:"id"`
    ModuleID      int64          `json:"module_id"`
    DateFilterCol string         `json:"date_filter_col"`
    ReportOption  string         `json:"report_option"`
    DynamicReport bool           `json:"dynamic_report"`
    TableName     string         `json:"table_name"`
    Icon          string         `json:"icon"`
    ShowSR        bool           `json:"show_sr"`
    Title         string         `json:"title"`
    Subtitle      string         `json:"subtitle"`
    Total         string         `json:"total"`
    Columns       []ReportColumn `json:"columns"`
}
