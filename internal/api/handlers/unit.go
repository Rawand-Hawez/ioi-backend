package handlers

import (
	"context"
	"errors"
	"log"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

func ListUnits(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	var businessEntityID pgtype.UUID
	if c.Query("business_entity_id") != "" {
		id, err := uuid.Parse(c.Query("business_entity_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
		}
		businessEntityID = toPgUUID(id)
	}

	var projectID pgtype.UUID
	if c.Query("project_id") != "" {
		id, err := uuid.Parse(c.Query("project_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
		}
		projectID = toPgUUID(id)
	}

	var structureNodeID pgtype.UUID
	if c.Query("structure_node_id") != "" {
		id, err := uuid.Parse(c.Query("structure_node_id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid structure_node_id"})
		}
		structureNodeID = toPgUUID(id)
	}

	var inventoryStatus pgtype.Text
	if c.Query("inventory_status") != "" {
		inventoryStatus = pgtype.Text{String: c.Query("inventory_status"), Valid: true}
	}

	var salesStatus pgtype.Text
	if c.Query("sales_status") != "" {
		salesStatus = pgtype.Text{String: c.Query("sales_status"), Valid: true}
	}

	var isActive pgtype.Bool
	if c.Query("is_active") != "" {
		v := c.Query("is_active") == "true"
		isActive = pgtype.Bool{Bool: v, Valid: true}
	}

	var total int64
	var items []db.Unit

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountUnits(ctx, db.CountUnitsParams{
			BusinessEntityID: businessEntityID,
			ProjectID:        projectID,
			StructureNodeID:  structureNodeID,
			InventoryStatus:  inventoryStatus,
			SalesStatus:      salesStatus,
			IsActive:         isActive,
		})
		if err != nil {
			return err
		}

		items, err = q.ListUnits(ctx, db.ListUnitsParams{
			BusinessEntityID: businessEntityID,
			ProjectID:        projectID,
			StructureNodeID:  structureNodeID,
			InventoryStatus:  inventoryStatus,
			SalesStatus:      salesStatus,
			IsActive:         isActive,
			Limit:            int32(perPage),
			Offset:           int32(offset),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Unit{}
	}

	return c.JSON(fiber.Map{
		"data": items,
		"pagination": fiber.Map{
			"page":        page,
			"per_page":    perPage,
			"total":       total,
			"total_pages": int(math.Ceil(float64(total) / float64(perPage))),
		},
	})
}

func GetUnit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var entity db.Unit

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.GetUnit(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "unit not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func CreateUnit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	var req struct {
		BusinessEntityID      string  `json:"business_entity_id" validate:"required"`
		BranchID              string  `json:"branch_id" validate:"required"`
		ProjectID             string  `json:"project_id" validate:"required"`
		StructureNodeID       *string `json:"structure_node_id"`
		ParentUnitID          *string `json:"parent_unit_id"`
		UnitType              string  `json:"unit_type" validate:"required,oneof=apartment villa retail office parking storage townhouse"`
		CommercialDisposition string  `json:"commercial_disposition" validate:"required,oneof=sale_only rent_only sale_or_rent internal_use inactive"`
		UnitCode              string  `json:"unit_code" validate:"required,min=1,max=30"`
		DisplayCode           *string `json:"display_code" validate:"omitempty,max=30"`
		UnitNo                *string `json:"unit_no" validate:"omitempty,max=20"`
		FloorValue            *string `json:"floor_value" validate:"omitempty,max=10"`
		FloorSortValue        *int32  `json:"floor_sort_value"`
		SectionNo             *string `json:"section_no" validate:"omitempty,max=10"`
		EntranceNo            *string `json:"entrance_no" validate:"omitempty,max=10"`
		BedroomCount          *int16  `json:"bedroom_count"`
		BathroomCount         *int16  `json:"bathroom_count"`
		AreaGrossSqm          *string `json:"area_gross_sqm"`
		AreaNetSqm            *string `json:"area_net_sqm"`
		AreaChargeableSqm     *string `json:"area_chargeable_sqm"`
		LandAreaSqm           *string `json:"land_area_sqm"`
		FacingDirection       *string `json:"facing_direction" validate:"omitempty,max=20"`
		InventoryStatus       string  `json:"inventory_status" validate:"omitempty,oneof=available reserved sold leased owner_occupied inactive"`
		SalesStatus           string  `json:"sales_status" validate:"omitempty,oneof=available reserved sold not_for_sale"`
		OccupancyStatus       string  `json:"occupancy_status" validate:"omitempty,oneof=vacant occupied owner_occupied"`
		MaintenanceStatus     string  `json:"maintenance_status" validate:"omitempty,oneof=none under_maintenance restricted"`
		ListPriceAmount       *string `json:"list_price_amount"`
		ListPriceCurrency     string  `json:"list_price_currency" validate:"omitempty,len=3"`
		ValuationAmount       *string `json:"valuation_amount"`
		MetadataJson          []byte  `json:"metadata_json"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.InventoryStatus == "" {
		req.InventoryStatus = "available"
	}
	if req.SalesStatus == "" {
		req.SalesStatus = "available"
	}
	if req.OccupancyStatus == "" {
		req.OccupancyStatus = "vacant"
	}
	if req.MaintenanceStatus == "" {
		req.MaintenanceStatus = "none"
	}
	if req.ListPriceCurrency == "" {
		req.ListPriceCurrency = "USD"
	}

	beID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
	}

	branchID, err := uuid.Parse(req.BranchID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid branch_id"})
	}

	var structureNodeID pgtype.UUID
	if req.StructureNodeID != nil {
		id, err := uuid.Parse(*req.StructureNodeID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid structure_node_id"})
		}
		structureNodeID = toPgUUID(id)
	}

	var parentUnitID pgtype.UUID
	if req.ParentUnitID != nil {
		id, err := uuid.Parse(*req.ParentUnitID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid parent_unit_id"})
		}
		parentUnitID = toPgUUID(id)
	}

	var entity db.Unit

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.CreateUnit(ctx, db.CreateUnitParams{
			BusinessEntityID:      toPgUUID(beID),
			BranchID:              toPgUUID(branchID),
			ProjectID:             toPgUUID(projectID),
			StructureNodeID:       structureNodeID,
			ParentUnitID:          parentUnitID,
			UnitType:              req.UnitType,
			CommercialDisposition: req.CommercialDisposition,
			UnitCode:              req.UnitCode,
			DisplayCode:           toPgText(req.DisplayCode),
			UnitNo:                toPgText(req.UnitNo),
			FloorValue:            toPgText(req.FloorValue),
			FloorSortValue:        toPgInt4(req.FloorSortValue),
			SectionNo:             toPgText(req.SectionNo),
			EntranceNo:            toPgText(req.EntranceNo),
			BedroomCount:          toPgInt2(req.BedroomCount),
			BathroomCount:         toPgInt2(req.BathroomCount),
			AreaGrossSqm:          toPgNumeric(req.AreaGrossSqm),
			AreaNetSqm:            toPgNumeric(req.AreaNetSqm),
			AreaChargeableSqm:     toPgNumeric(req.AreaChargeableSqm),
			LandAreaSqm:           toPgNumeric(req.LandAreaSqm),
			FacingDirection:       toPgText(req.FacingDirection),
			InventoryStatus:       req.InventoryStatus,
			SalesStatus:           req.SalesStatus,
			OccupancyStatus:       req.OccupancyStatus,
			MaintenanceStatus:     req.MaintenanceStatus,
			ListPriceAmount:       toPgNumeric(req.ListPriceAmount),
			ListPriceCurrency:     req.ListPriceCurrency,
			ValuationAmount:       toPgNumeric(req.ValuationAmount),
			MetadataJson:          req.MetadataJson,
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "unit_code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(entity)
}

func UpdateUnit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		StructureNodeID       *string `json:"structure_node_id"`
		ParentUnitID          *string `json:"parent_unit_id"`
		UnitType              *string `json:"unit_type" validate:"omitempty,oneof=apartment villa retail office parking storage townhouse"`
		CommercialDisposition *string `json:"commercial_disposition" validate:"omitempty,oneof=sale_only rent_only sale_or_rent internal_use inactive"`
		DisplayCode           *string `json:"display_code" validate:"omitempty,max=30"`
		UnitNo                *string `json:"unit_no" validate:"omitempty,max=20"`
		FloorValue            *string `json:"floor_value" validate:"omitempty,max=10"`
		FloorSortValue        *int32  `json:"floor_sort_value"`
		SectionNo             *string `json:"section_no" validate:"omitempty,max=10"`
		EntranceNo            *string `json:"entrance_no" validate:"omitempty,max=10"`
		BedroomCount          *int16  `json:"bedroom_count"`
		BathroomCount         *int16  `json:"bathroom_count"`
		AreaGrossSqm          *string `json:"area_gross_sqm"`
		AreaNetSqm            *string `json:"area_net_sqm"`
		AreaChargeableSqm     *string `json:"area_chargeable_sqm"`
		LandAreaSqm           *string `json:"land_area_sqm"`
		FacingDirection       *string `json:"facing_direction" validate:"omitempty,max=20"`
		InventoryStatus       *string `json:"inventory_status" validate:"omitempty,oneof=available reserved sold leased owner_occupied inactive"`
		SalesStatus           *string `json:"sales_status" validate:"omitempty,oneof=available reserved sold not_for_sale"`
		OccupancyStatus       *string `json:"occupancy_status" validate:"omitempty,oneof=vacant occupied owner_occupied"`
		MaintenanceStatus     *string `json:"maintenance_status" validate:"omitempty,oneof=none under_maintenance restricted"`
		ListPriceAmount       *string `json:"list_price_amount"`
		ListPriceCurrency     *string `json:"list_price_currency" validate:"omitempty,len=3"`
		ValuationAmount       *string `json:"valuation_amount"`
		MetadataJson          []byte  `json:"metadata_json"`
		IsActive              *bool   `json:"is_active"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var structureNodeID pgtype.UUID
	if req.StructureNodeID != nil {
		id, err := uuid.Parse(*req.StructureNodeID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid structure_node_id"})
		}
		structureNodeID = toPgUUID(id)
	}

	var parentUnitID pgtype.UUID
	if req.ParentUnitID != nil {
		id, err := uuid.Parse(*req.ParentUnitID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid parent_unit_id"})
		}
		parentUnitID = toPgUUID(id)
	}

	var entity db.Unit

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.UpdateUnit(ctx, db.UpdateUnitParams{
			ID:                    toPgUUID(id),
			StructureNodeID:       structureNodeID,
			ParentUnitID:          parentUnitID,
			UnitType:              toPgText(req.UnitType),
			CommercialDisposition: toPgText(req.CommercialDisposition),
			DisplayCode:           toPgText(req.DisplayCode),
			UnitNo:                toPgText(req.UnitNo),
			FloorValue:            toPgText(req.FloorValue),
			FloorSortValue:        toPgInt4(req.FloorSortValue),
			SectionNo:             toPgText(req.SectionNo),
			EntranceNo:            toPgText(req.EntranceNo),
			BedroomCount:          toPgInt2(req.BedroomCount),
			BathroomCount:         toPgInt2(req.BathroomCount),
			AreaGrossSqm:          toPgNumeric(req.AreaGrossSqm),
			AreaNetSqm:            toPgNumeric(req.AreaNetSqm),
			AreaChargeableSqm:     toPgNumeric(req.AreaChargeableSqm),
			LandAreaSqm:           toPgNumeric(req.LandAreaSqm),
			FacingDirection:       toPgText(req.FacingDirection),
			InventoryStatus:       toPgText(req.InventoryStatus),
			SalesStatus:           toPgText(req.SalesStatus),
			OccupancyStatus:       toPgText(req.OccupancyStatus),
			MaintenanceStatus:     toPgText(req.MaintenanceStatus),
			ListPriceAmount:       toPgNumeric(req.ListPriceAmount),
			ListPriceCurrency:     toPgText(req.ListPriceCurrency),
			ValuationAmount:       toPgNumeric(req.ValuationAmount),
			MetadataJson:          req.MetadataJson,
			IsActive:              toPgBool(req.IsActive),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "unit not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func DeactivateUnit(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		return q.DeactivateUnit(ctx, toPgUUID(id))
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.SendStatus(204)
}

func UpdateUnitCode(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeout)
	defer cancel()

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req struct {
		UnitCode    string  `json:"unit_code" validate:"required,min=1,max=30"`
		DisplayCode *string `json:"display_code" validate:"omitempty,max=30"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var entity db.Unit

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.UpdateUnitCode(ctx, db.UpdateUnitCodeParams{
			ID:          toPgUUID(id),
			UnitCode:    req.UnitCode,
			DisplayCode: toPgText(req.DisplayCode),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "unit not found"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "unit_code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func toPgInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func toPgInt2(v *int16) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{Valid: false}
	}
	return pgtype.Int2{Int16: *v, Valid: true}
}

func toPgNumeric(v *string) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{Valid: false}
	}
	var n pgtype.Numeric
	if err := n.Scan(*v); err != nil {
		return pgtype.Numeric{Valid: false}
	}
	return n
}
