// # Saga Compensation Example — Hotel/Flight/Car Booking
//
// Demonstrates the Saga pattern: a distributed transaction where each step
// has a corresponding compensation that is automatically invoked in reverse
// order if a later step fails.
//
// Scenario: booking a travel package (hotel + flight + car).
//
//   Happy path:  IDLE → BOOKING → CONFIRMED
//   Failure path (car unavailable):
//     1. Hotel reserved  ✓
//     2. Flight reserved ✓
//     3. Car reserved    ✗  ← fails
//        ← Flight cancelled (compensation for step 2)
//        ← Hotel cancelled  (compensation for step 1)
//     State stays at BOOKING (saga rolled back)
//
// Key API shown:
//
//   .Saga(activity, compensation)   — register a step + its undo function
//   Automatic reverse compensation  — no user code required on failure
//
// Run:
//
//	go run ./examples/saga
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/im-adarsh/go-statemachine/workflow"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain
// ─────────────────────────────────────────────────────────────────────────────

type Booking struct {
	ID           string
	Destination  string
	HotelRef     string
	FlightRef    string
	CarRef       string
	CarAvailable bool // simulates external inventory
}

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel errors
// ─────────────────────────────────────────────────────────────────────────────

var ErrCarUnavailable = errors.New("no cars available at destination")

// ─────────────────────────────────────────────────────────────────────────────
// Step 1: Hotel
// ─────────────────────────────────────────────────────────────────────────────

func reserveHotel(_ context.Context, b *Booking) error {
	b.HotelRef = fmt.Sprintf("HTL-%s-42", b.Destination)
	logStep("RESERVE", "hotel", b.HotelRef, nil)
	return nil
}

func cancelHotel(_ context.Context, b *Booking) error {
	logStep("CANCEL ", "hotel", b.HotelRef, nil)
	b.HotelRef = ""
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 2: Flight
// ─────────────────────────────────────────────────────────────────────────────

func reserveFlight(_ context.Context, b *Booking) error {
	b.FlightRef = fmt.Sprintf("FLT-%s-789", b.Destination)
	logStep("RESERVE", "flight", b.FlightRef, nil)
	return nil
}

func cancelFlight(_ context.Context, b *Booking) error {
	logStep("CANCEL ", "flight", b.FlightRef, nil)
	b.FlightRef = ""
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 3: Car
// ─────────────────────────────────────────────────────────────────────────────

func reserveCar(_ context.Context, b *Booking) error {
	if !b.CarAvailable {
		logStep("RESERVE", "car", "", ErrCarUnavailable)
		return ErrCarUnavailable
	}
	b.CarRef = fmt.Sprintf("CAR-%s-001", b.Destination)
	logStep("RESERVE", "car", b.CarRef, nil)
	return nil
}

func cancelCar(_ context.Context, b *Booking) error {
	// Only called if reserveCar succeeded (which it didn't in the failure scenario).
	logStep("CANCEL ", "car", b.CarRef, nil)
	b.CarRef = ""
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Workflow definition
// ─────────────────────────────────────────────────────────────────────────────

var travelWF = workflow.Define[*Booking]().
	// Book all three services as Saga steps.
	// If any step fails, the already-completed steps are automatically compensated
	// in reverse order — no explicit rollback logic required.
	From("IDLE").On("book").To("CONFIRMED").
	Saga(reserveHotel, cancelHotel).
	Saga(reserveFlight, cancelFlight).
	Saga(reserveCar, cancelCar).

	// Explicit cancel signal (full booking confirmed, then later cancelled by user).
	From("CONFIRMED").On("cancel").To("CANCELLED").
	Activity(cancelCar).
	Activity(cancelFlight).
	Activity(cancelHotel).

	MustBuild()

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	ctx := context.Background()

	fmt.Println("┌─ Travel Workflow graph ─────────────────────────────────────")
	fmt.Print(travelWF.Visualize())
	fmt.Println("└────────────────────────────────────────────────────────────")
	fmt.Println()

	// ── Scenario A: happy path — car available ────────────────────────────────
	fmt.Println("╔══ Scenario A: All services available (happy path) ══")
	bookingA := &Booking{ID: "BKG-001", Destination: "PAR", CarAvailable: true}
	runScenario(ctx, bookingA, []string{"book"})

	// ── Scenario B: car unavailable — hotel and flight compensated ────────────
	fmt.Println("╔══ Scenario B: Car unavailable — saga rolls back ══")
	bookingB := &Booking{ID: "BKG-002", Destination: "NYC", CarAvailable: false}
	runScenario(ctx, bookingB, []string{"book"})

	// ── Scenario C: successful booking then user cancels ─────────────────────
	fmt.Println("╔══ Scenario C: Confirmed booking then user cancels ══")
	bookingC := &Booking{ID: "BKG-003", Destination: "TYO", CarAvailable: true}
	runScenario(ctx, bookingC, []string{"book", "cancel"})
}

func runScenario(ctx context.Context, b *Booking, signals []string) {
	exec := travelWF.NewExecution(ctx, "IDLE",
		workflow.WithHooks(workflow.ExecutionHooks[*Booking]{
			OnTransition: func(_ context.Context, from, to, sig string, b *Booking) {
				fmt.Printf("  → [%s] %s --%s--> %s\n", b.ID, from, sig, to)
			},
			OnError: func(_ context.Context, state, sig string, err error, b *Booking) {
				fmt.Printf("  ✗ [%s] signal=%s state=%s error=%v\n", b.ID, sig, state, err)
			},
		}),
	)

	for _, sig := range signals {
		fmt.Printf("\n  Sending signal: %q\n", sig)
		if err := exec.Signal(ctx, sig, b); err != nil {
			fmt.Printf("  Signal failed (saga may have compensated): %v\n", err)
		}
	}

	fmt.Printf("\n  Final state : %s\n", exec.CurrentState())
	fmt.Printf("  Hotel ref   : %q\n", b.HotelRef)
	fmt.Printf("  Flight ref  : %q\n", b.FlightRef)
	fmt.Printf("  Car ref     : %q\n", b.CarRef)

	fmt.Println("\n  Event history:")
	fmt.Printf("  %-4s  %-10s  %-8s  %-12s  %s\n", "#", "From", "Signal", "To", "Error")
	fmt.Println("  " + strings.Repeat("─", 50))
	for i, e := range exec.History() {
		errStr := "—"
		if e.Err != nil {
			errStr = truncate(e.Err.Error(), 30)
		}
		fmt.Printf("  %-4d  %-10s  %-8s  %-12s  %s\n",
			i+1, e.FromState, e.Signal, e.ToState, errStr)
	}
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func logStep(action, service, ref string, err error) {
	ts := time.Now().Format("15:04:05.000")
	if err != nil {
		fmt.Printf("  [%s] %-7s %-8s ✗ %v\n", ts, action, service, err)
	} else {
		fmt.Printf("  [%s] %-7s %-8s ✓ ref=%s\n", ts, action, service, ref)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
