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

func ListParties(c *fiber.Ctx) error {
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

	var partyType pgtype.Text
	if c.Query("party_type") != "" {
		partyType = pgtype.Text{String: c.Query("party_type"), Valid: true}
	}

	var status pgtype.Text
	if c.Query("status") != "" {
		status = pgtype.Text{String: c.Query("status"), Valid: true}
	}

	var total int64
	var items []db.Party

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		total, err = q.CountParties(ctx, db.CountPartiesParams{
			PartyType: partyType,
			Status:    status,
		})
		if err != nil {
			return err
		}

		items, err = q.ListParties(ctx, db.ListPartiesParams{
			PartyType: partyType,
			Status:    status,
			Limit:     int32(perPage),
			Offset:    int32(offset),
		})
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.Party{}
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

func GetParty(c *fiber.Ctx) error {
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

	var entity db.Party

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.GetParty(ctx, toPgUUID(id))
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "party not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}

func CreateParty(c *fiber.Ctx) error {
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
		PartyType         string  `json:"party_type" validate:"required,oneof=person organization"`
		PartyCode         string  `json:"party_code" validate:"required"`
		DisplayName       string  `json:"display_name" validate:"required"`
		FullName          *string `json:"full_name"`
		FirstName         *string `json:"first_name"`
		MiddleName        *string `json:"middle_name"`
		LastName          *string `json:"last_name"`
		OrganizationName  *string `json:"organization_name"`
		PrimaryPhone      string  `json:"primary_phone" validate:"required"`
		SecondaryPhone    *string `json:"secondary_phone"`
		PrimaryEmail      *string `json:"primary_email"`
		DateOfBirth       *string `json:"date_of_birth"`
		Nationality       *string `json:"nationality"`
		NationalIDNo      *string `json:"national_id_no"`
		PassportNo        *string `json:"passport_no"`
		RegistrationNo    *string `json:"registration_no"`
		TaxNo             *string `json:"tax_no"`
		PreferredLanguage string  `json:"preferred_language"`
		LegacyRef         *string `json:"legacy_ref"`
		Notes             *string `json:"notes"`
		Status            string  `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.PartyType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "party_type is required"})
	}
	if req.PartyType != "person" && req.PartyType != "organization" {
		return c.Status(400).JSON(fiber.Map{"error": "party_type must be 'person' or 'organization'"})
	}
	if req.PartyCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "party_code is required"})
	}
	if req.DisplayName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "display_name is required"})
	}
	if req.PrimaryPhone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "primary_phone is required"})
	}

	if req.PreferredLanguage == "" {
		req.PreferredLanguage = "ar"
	}

	if req.Status == "" {
		req.Status = "active"
	}

	var dateOfBirth pgtype.Date
	if req.DateOfBirth != nil {
		dateOfBirth = parseDate(*req.DateOfBirth)
	}

	var entity db.Party

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		entity, err = q.CreateParty(ctx, db.CreatePartyParams{
			PartyType:         req.PartyType,
			PartyCode:         req.PartyCode,
			DisplayName:       req.DisplayName,
			FullName:          toPgText(req.FullName),
			FirstName:         toPgText(req.FirstName),
			MiddleName:        toPgText(req.MiddleName),
			LastName:          toPgText(req.LastName),
			OrganizationName:  toPgText(req.OrganizationName),
			PrimaryPhone:      req.PrimaryPhone,
			SecondaryPhone:    toPgText(req.SecondaryPhone),
			PrimaryEmail:      toPgText(req.PrimaryEmail),
			DateOfBirth:       dateOfBirth,
			Nationality:       toPgText(req.Nationality),
			NationalIDNo:      toPgText(req.NationalIDNo),
			PassportNo:        toPgText(req.PassportNo),
			RegistrationNo:    toPgText(req.RegistrationNo),
			TaxNo:             toPgText(req.TaxNo),
			PreferredLanguage: req.PreferredLanguage,
			LegacyRef:         toPgText(req.LegacyRef),
			Notes:             toPgText(req.Notes),
			Status:            req.Status,
		})
		return err
	})

	if err != nil {
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "party_code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(entity)
}

func UpdateParty(c *fiber.Ctx) error {
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
		PartyType         *string `json:"party_type" validate:"omitempty,oneof=person organization"`
		DisplayName       *string `json:"display_name"`
		FullName          *string `json:"full_name"`
		FirstName         *string `json:"first_name"`
		MiddleName        *string `json:"middle_name"`
		LastName          *string `json:"last_name"`
		OrganizationName  *string `json:"organization_name"`
		PrimaryPhone      *string `json:"primary_phone"`
		SecondaryPhone    *string `json:"secondary_phone"`
		PrimaryEmail      *string `json:"primary_email"`
		DateOfBirth       *string `json:"date_of_birth"`
		Nationality       *string `json:"nationality"`
		NationalIDNo      *string `json:"national_id_no"`
		PassportNo        *string `json:"passport_no"`
		RegistrationNo    *string `json:"registration_no"`
		TaxNo             *string `json:"tax_no"`
		PreferredLanguage *string `json:"preferred_language"`
		LegacyRef         *string `json:"legacy_ref"`
		Notes             *string `json:"notes"`
		Status            *string `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	var dateOfBirth pgtype.Date
	if req.DateOfBirth != nil {
		dateOfBirth = parseDate(*req.DateOfBirth)
	}

	var entity db.Party

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		entity, err = q.UpdateParty(ctx, db.UpdatePartyParams{
			ID:                toPgUUID(id),
			PartyType:         toPgText(req.PartyType),
			DisplayName:       toPgText(req.DisplayName),
			FullName:          toPgText(req.FullName),
			FirstName:         toPgText(req.FirstName),
			MiddleName:        toPgText(req.MiddleName),
			LastName:          toPgText(req.LastName),
			OrganizationName:  toPgText(req.OrganizationName),
			PrimaryPhone:      toPgText(req.PrimaryPhone),
			SecondaryPhone:    toPgText(req.SecondaryPhone),
			PrimaryEmail:      toPgText(req.PrimaryEmail),
			DateOfBirth:       dateOfBirth,
			Nationality:       toPgText(req.Nationality),
			NationalIDNo:      toPgText(req.NationalIDNo),
			PassportNo:        toPgText(req.PassportNo),
			RegistrationNo:    toPgText(req.RegistrationNo),
			TaxNo:             toPgText(req.TaxNo),
			PreferredLanguage: toPgText(req.PreferredLanguage),
			LegacyRef:         toPgText(req.LegacyRef),
			Notes:             toPgText(req.Notes),
			Status:            toPgText(req.Status),
		})
		return err
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "party not found"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(entity)
}
