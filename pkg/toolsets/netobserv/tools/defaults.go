package tools

const (
	DefaultTimeRangeSeconds   = 300
	DefaultLimit              = 100
	DefaultRecordType         = "flowLog"
	DefaultPacketLoss         = "all"
	DefaultDataSource         = "auto"
	DefaultMetricType         = "Bytes"
	DefaultMetricFunction     = "rate"
	DefaultRateInterval       = "1m"
	DefaultStep               = "30s"
	DefaultExportFormat       = "csv"
	DefaultExportMaxBodyBytes = 2 << 20 // 2 MiB
)
