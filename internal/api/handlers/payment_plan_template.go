package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"IOI-real-estate-backend/internal/api/middleware"
	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"
)

const dbTimeoutPaymentPlan = 5 * time.Second

type paymentPlanGenerationRule struct {
	DownPayment paymentPlanRuleSegment   `json:"down_payment"`
	Tranches    []paymentPlanRuleSegment `json:"tranches"`
}

type paymentPlanRuleSegment struct {
	Percentage       paymentPlanPercentage `json:"percentage"`
	Anchor           string                `json:"anchor"`
	OffsetDays       int32                 `json:"offset_days"`
	InstallmentCount int32                 `json:"installment_count"`
	Frequency        string                `json:"frequency"`
	LineType         string                `json:"line_type"`
}

type paymentPlanPercentage struct {
	value *big.Rat
}

func (p *paymentPlanPercentage) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return errors.New("percentage is required")
	}
	if strings.HasPrefix(raw, `"`) {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		raw = strings.TrimSpace(text)
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return fmt.Errorf("invalid percentage")
	}
	p.value = value
	return nil
}

func validatePaymentPlanGenerationRule(raw json.RawMessage, installmentCount int32) error {
	if len(raw) == 0 {
		return errors.New("generation_rule_json is required")
	}
	if installmentCount <= 0 {
		return errors.New("installment_count must be positive")
	}

	var rule paymentPlanGenerationRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return fmt.Errorf("invalid generation_rule_json")
	}

	totalPercentage := new(big.Rat)
	if err := validatePaymentPlanRuleSegment("down_payment", rule.DownPayment, false); err != nil {
		return err
	}
	totalPercentage.Add(totalPercentage, rule.DownPayment.Percentage.value)

	if len(rule.Tranches) == 0 {
		return errors.New("at least one tranche is required")
	}

	var totalInstallments int32
	for i, tranche := range rule.Tranches {
		if err := validatePaymentPlanRuleSegment(fmt.Sprintf("tranches[%d]", i), tranche, true); err != nil {
			return err
		}
		totalPercentage.Add(totalPercentage, tranche.Percentage.value)
		totalInstallments += tranche.InstallmentCount
	}

	if totalPercentage.Cmp(big.NewRat(100, 1)) != 0 {
		return errors.New("generation_rule_json percentages must total 100")
	}
	if totalInstallments != installmentCount {
		return errors.New("tranche installment_count total must equal template installment_count")
	}
	return nil
}

func validatePaymentPlanRuleSegment(name string, segment paymentPlanRuleSegment, tranche bool) error {
	if segment.Percentage.value == nil {
		return fmt.Errorf("%s.percentage is required", name)
	}
	if segment.Percentage.value.Cmp(big.NewRat(0, 1)) <= 0 {
		return fmt.Errorf("%s.percentage must be positive", name)
	}
	if !isAllowedPaymentPlanAnchor(segment.Anchor) {
		return fmt.Errorf("%s.anchor is invalid", name)
	}
	if tranche {
		if segment.InstallmentCount <= 0 {
			return fmt.Errorf("%s.installment_count must be positive", name)
		}
		if segment.InstallmentCount > 1 && segment.Frequency == "" {
			return fmt.Errorf("%s.frequency is required", name)
		}
	}
	if segment.Frequency != "" && !isAllowedPaymentPlanFrequency(segment.Frequency) {
		return fmt.Errorf("%s.frequency is invalid", name)
	}
	return nil
}

func isAllowedPaymentPlanAnchor(anchor string) bool {
	switch anchor {
	case "reservation", "contract_date", "handover_date":
		return true
	default:
		return false
	}
}

func isAllowedPaymentPlanFrequency(frequency string) bool {
	switch frequency {
	case "once", "monthly", "quarterly", "semiannual", "annual":
		return true
	default:
		return false
	}
}

// ListPaymentPlanTemplates handles GET /payment-plan-templates
func ListPaymentPlanTemplates(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeoutPaymentPlan)
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

	listParams := db.ListPaymentPlanTemplatesParams{
		Limit:  int32(perPage),
		Offset: int32(offset),
	}

	if beID := c.Query("business_entity_id"); beID != "" {
		if id, err := uuid.Parse(beID); err == nil {
			listParams.BusinessEntityID = toPgUUID(id)
		}
	}
	if projectID := c.Query("project_id"); projectID != "" {
		if id, err := uuid.Parse(projectID); err == nil {
			listParams.ProjectID = toPgUUID(id)
		}
	}
	if status := c.Query("status"); status != "" {
		listParams.Status = toPgText(&status)
	}

	var total int64
	var items []db.PaymentPlanTemplate

	err := p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error

		countParams := db.CountPaymentPlanTemplatesParams{
			BusinessEntityID: listParams.BusinessEntityID,
			ProjectID:        listParams.ProjectID,
			Status:           listParams.Status,
		}
		total, err = q.CountPaymentPlanTemplates(ctx, countParams)
		if err != nil {
			return err
		}

		items, err = q.ListPaymentPlanTemplates(ctx, listParams)
		return err
	})

	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	if items == nil {
		items = []db.PaymentPlanTemplate{}
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

// CreatePaymentPlanTemplate handles POST /payment-plan-templates
func CreatePaymentPlanTemplate(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), dbTimeoutPaymentPlan)
	defer cancel()

	var req struct {
		BusinessEntityID   string          `json:"business_entity_id"`
		ProjectID          *string         `json:"project_id"`
		Code               string          `json:"code"`
		Name               string          `json:"name"`
		Status             string          `json:"status"`
		FrequencyType      string          `json:"frequency_type"`
		InstallmentCount   int32           `json:"installment_count"`
		GenerationRuleJSON json.RawMessage `json:"generation_rule_json"`
		Notes              *string         `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	businessEntityID, err := uuid.Parse(req.BusinessEntityID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business_entity_id"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	if len(req.GenerationRuleJSON) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "generation_rule_json is required"})
	}
	if err := validatePaymentPlanGenerationRule(req.GenerationRuleJSON, req.InstallmentCount); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var template db.PaymentPlanTemplate

	err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
		q := db.New(tx)

		createParams := db.CreatePaymentPlanTemplateParams{
			BusinessEntityID:   toPgUUID(businessEntityID),
			Code:               req.Code,
			Name:               req.Name,
			Status:             req.Status,
			FrequencyType:      req.FrequencyType,
			InstallmentCount:   req.InstallmentCount,
			GenerationRuleJson: req.GenerationRuleJSON,
			Notes:              toPgText(req.Notes),
		}

		if req.ProjectID != nil {
			projectID, err := uuid.Parse(*req.ProjectID)
			if err != nil {
				return errors.New("invalid project_id")
			}
			createParams.ProjectID = toPgUUID(projectID)
		}

		template, err = q.CreatePaymentPlanTemplate(ctx, createParams)
		return err
	})

	if err != nil {
		if err.Error() == "invalid project_id" {
			return c.Status(400).JSON(fiber.Map{"error": "invalid project_id"})
		}
		if isDuplicateKeyError(err) {
			return c.Status(409).JSON(fiber.Map{"error": "code already exists"})
		}
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(201).JSON(template)
}
