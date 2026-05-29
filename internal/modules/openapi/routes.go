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
	keyed.GET("/active_members_renewal_range_past", ctrl.ActiveMembersRenewalRangePast)
	keyed.GET("/active_members_renewal_range_current", ctrl.ActiveMembersRenewalRangeCurrent)
}
