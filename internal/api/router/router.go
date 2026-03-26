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
}
