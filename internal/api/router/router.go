package router

import (
	"IOI-real-estate-backend/internal/api/handlers"
	"IOI-real-estate-backend/internal/api/middleware"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes organizes all the custom API endpoints for the Go backend.
func SetupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Public demo endpoint
	api.Get("/demo", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Custom Fiber JSON response from IOI Backend API",
		})
	})

	// Business Structure routes
	be := api.Group("/business-entities", middleware.RequireAuth(), middleware.InjectGUCVariables())
	be.Get("/", handlers.ListBusinessEntities)
	be.Post("/", handlers.CreateBusinessEntity)
	be.Get("/:id", handlers.GetBusinessEntity)
	be.Patch("/:id", handlers.UpdateBusinessEntity)
	be.Get("/:id/branches", handlers.ListBranches)

	branches := api.Group("/branches", middleware.RequireAuth(), middleware.InjectGUCVariables())
	branches.Post("/", handlers.CreateBranch)
	branches.Get("/:id", handlers.GetBranch)
	branches.Patch("/:id", handlers.UpdateBranch)

	projects := api.Group("/projects", middleware.RequireAuth(), middleware.InjectGUCVariables())
	projects.Get("/", handlers.ListProjects)
	projects.Post("/", handlers.CreateProject)
	projects.Get("/:id", handlers.GetProject)
	projects.Patch("/:id", handlers.UpdateProject)
	projects.Get("/:id/structure-nodes", handlers.ListStructureNodes)

	structureNodes := api.Group("/structure-nodes", middleware.RequireAuth(), middleware.InjectGUCVariables())
	structureNodes.Get("/", handlers.ListStructureNodes)
	structureNodes.Post("/", handlers.CreateStructureNode)
	structureNodes.Get("/:id", handlers.GetStructureNode)
	structureNodes.Patch("/:id", handlers.UpdateStructureNode)

	units := api.Group("/units", middleware.RequireAuth(), middleware.InjectGUCVariables())
	units.Get("/", handlers.ListUnits)
	units.Post("/", handlers.CreateUnit)
	units.Get("/:id", handlers.GetUnit)
	units.Patch("/:id", handlers.UpdateUnit)
	units.Post("/:id/update-code", handlers.UpdateUnitCode)
	units.Get("/:unit_id/ownerships", handlers.ListUnitOwnerships)
	units.Post("/:unit_id/ownerships", handlers.CreateUnitOwnership)
	units.Get("/:unit_id/responsibilities", handlers.ListResponsibilityAssignments)
	units.Post("/:unit_id/responsibilities", handlers.CreateResponsibilityAssignment)

	// Phase 3: Party domain routes
	parties := api.Group("/parties", middleware.RequireAuth(), middleware.InjectGUCVariables())
	parties.Get("/", handlers.ListParties)
	parties.Post("/", handlers.CreateParty)
	parties.Get("/:id", handlers.GetParty)
	parties.Patch("/:id", handlers.UpdateParty)

	// Phase 3: Unit ownership close route
	unitOwnerships := api.Group("/unit-ownerships", middleware.RequireAuth(), middleware.InjectGUCVariables())
	unitOwnerships.Post("/:id/close", handlers.CloseUnitOwnership)

	// Phase 3: Responsibility assignment close route
	responsibilityAssignments := api.Group("/responsibility-assignments", middleware.RequireAuth(), middleware.InjectGUCVariables())
	responsibilityAssignments.Post("/:id/close", handlers.CloseResponsibilityAssignment)

	// Phase 4: Authorization domain routes
	roles := api.Group("/roles", middleware.RequireAuth(), middleware.InjectGUCVariables())
	roles.Get("/", handlers.ListRoles)
	roles.Get("/:id/permissions", handlers.ListRolePermissions)

	permissions := api.Group("/permissions", middleware.RequireAuth(), middleware.InjectGUCVariables())
	permissions.Get("/", handlers.ListPermissions)

	users := api.Group("/users", middleware.RequireAuth(), middleware.InjectGUCVariables())
	users.Get("/:id/role-assignments", handlers.ListUserRoleAssignments)
	users.Post("/:id/role-assignments", handlers.AssignRoleToUser)

	userRoleAssignments := api.Group("/user-role-assignments", middleware.RequireAuth(), middleware.InjectGUCVariables())
	userRoleAssignments.Get("/:id", handlers.GetUserRoleAssignment)
	userRoleAssignments.Delete("/:id", handlers.RemoveRoleFromUser)

	me := api.Group("/me", middleware.RequireAuth(), middleware.InjectGUCVariables())
	me.Get("/permissions", handlers.GetMyPermissions)

	// Phase 5: Shared Finance routes
	receivables := api.Group("/receivables", middleware.RequireAuth(), middleware.InjectGUCVariables())
	receivables.Get("/", handlers.ListReceivables)
	receivables.Get("/:id", handlers.GetReceivable)
	receivables.Post("/", middleware.RequirePermission("finance.receivable.manual"), handlers.CreateReceivable)

	payments := api.Group("/payments", middleware.RequireAuth(), middleware.InjectGUCVariables())
	payments.Get("/", handlers.ListPayments)
	payments.Get("/:id", handlers.GetPayment)
	payments.Post("/", middleware.RequirePermission("finance.payment.create"), handlers.CreatePayment)
	payments.Post("/:id/post", middleware.RequirePermission("finance.payment.post"), handlers.PostPayment)
	payments.Post("/:id/allocate", middleware.RequirePermission("finance.payment.post"), handlers.AllocatePayment)
	payments.Post("/:id/void", middleware.RequirePermission("finance.payment.void"), handlers.VoidPayment)

	creditBalances := api.Group("/credit-balances", middleware.RequireAuth(), middleware.InjectGUCVariables())
	creditBalances.Get("/", handlers.ListCreditBalances)
	creditBalances.Post("/:id/apply", middleware.RequirePermission("finance.credit.apply"), handlers.ApplyCreditBalance)

	adjustments := api.Group("/financial-adjustments", middleware.RequireAuth(), middleware.InjectGUCVariables())
	adjustments.Get("/", handlers.ListFinancialAdjustments)
	adjustments.Get("/:id", handlers.GetFinancialAdjustment)
	adjustments.Post("/", middleware.RequirePermission("finance.adjustment.create"), handlers.CreateFinancialAdjustment)
	adjustments.Post("/:id/approve", middleware.RequirePermission("finance.adjustment.approve"), handlers.ApproveFinancialAdjustment)
	adjustments.Post("/:id/reject", middleware.RequirePermission("finance.adjustment.approve"), handlers.RejectFinancialAdjustment)

	// Statement routes
	parties.Get("/:id/statement", handlers.GetPartyStatement)
	units.Get("/:id/statement", handlers.GetUnitStatement)

	// Approval Policy routes
	approvalPolicies := api.Group("/approval-policies", middleware.RequireAuth(), middleware.InjectGUCVariables())
	approvalPolicies.Get("/", handlers.ListApprovalPolicies)
	approvalPolicies.Get("/:id", handlers.GetApprovalPolicy)
	approvalPolicies.Post("/", middleware.RequirePermission("approvals.policy.manage"), handlers.CreateApprovalPolicy)
	approvalPolicies.Patch("/:id", middleware.RequirePermission("approvals.policy.manage"), handlers.UpdateApprovalPolicy)

	// Approval Request routes
	approvalRequests := api.Group("/approval-requests", middleware.RequireAuth(), middleware.InjectGUCVariables())
	approvalRequests.Get("/", handlers.ListApprovalRequests)
	approvalRequests.Get("/:id", handlers.GetApprovalRequest)
	approvalRequests.Post("/", handlers.CreateApprovalRequest)
	approvalRequests.Post("/:id/decide", middleware.RequirePermission("approvals.request.decide"), handlers.DecideApprovalRequest)
	approvalRequests.Post("/:id/cancel", handlers.CancelApprovalRequest)

	// Audit Log routes
	auditLogs := api.Group("/audit-logs", middleware.RequireAuth(), middleware.InjectGUCVariables())
	auditLogs.Get("/", middleware.RequirePermission("audit.log.view"), handlers.ListAuditLogs)
	auditLogs.Get("/:entity_type/:entity_id", middleware.RequirePermission("audit.log.view"), handlers.GetAuditLogsForEntity)
}
