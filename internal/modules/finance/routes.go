package finance

import (
	"time"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
	"server_1/internal/router"
)

func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker) {
	g := v1.Group("/finance")

	ctl := &Controller{Repo: NewRepo()}
	g.POST("/franchisee-report", prism.RequirePrism(checker, "report:read"), ctl.FranchiseeReport)

	inv := &FranchiseInvoiceController{Repo: NewFranchiseInvoiceRepo()}
	invG := g.Group("/franchise-invoice", prism.RequirePrism(checker, "report:read"))
	invG.GET("/init", inv.GetFranchiseInvoiceInit)
	invG.POST("", inv.CreateFranchiseInvoice)
	invG.PUT("", inv.UpdateFranchiseInvoice)
	invG.DELETE("", inv.DeleteFranchiseInvoice)
	invG.GET("/sub-invoices", inv.ListSubInvoices)
	invG.POST("/sub-invoice", inv.CreateSubInvoice)
	invG.DELETE("/sub-invoice", inv.DeleteSubInvoice)
	invG.POST("/sales-invoice", inv.CreateSalesInvoice)
	invG.POST("/sales-invoice-document", inv.RegenerateSalesInvoiceDocument)
	invG.GET("/annexure", inv.GetMemberTransferAnnexure)
	invG.GET("/invoice-list", router.WithRequestTimeoutOverride(90*time.Second), inv.GetInvoiceList)
}
