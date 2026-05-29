package openapi

import "github.com/gin-gonic/gin"

func MountRoutes(v1 *gin.RouterGroup, apiKey string) {
	ctrl := NewController(NewRepo())

	open := v1.Group("/open")

	public := open.Group("/public")
	public.GET("/health", ctrl.Health)

	keyed := open.Group("/key")
	keyed.Use(RequireAPIKey(apiKey))
	keyed.GET("/inquiry-demo-followup", ctrl.InquiryDemoFollowup)
	keyed.GET("/inquiry_demo_followup", ctrl.InquiryDemoFollowup)
	keyed.GET("/get_current_data", ctrl.GetCurrentData)
	keyed.GET("/get_old_data", ctrl.GetOldData)
	keyed.GET("/revenue_report", ctrl.RevenueReport)
	keyed.GET("/get_demo_form_response_data", ctrl.GetDemoFormResponseData)
	keyed.GET("/active_members_renewal_range_past", ctrl.ActiveMembersRenewalRangePast)
	keyed.GET("/active_members_renewal_range_current", ctrl.ActiveMembersRenewalRangeCurrent)
}
