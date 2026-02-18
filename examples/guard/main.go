// Guard example: conditional transitions using guard conditions.
//
// Run with: go run ./examples/guard/main.go
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/im-adarsh/go-statemachine/statemachine"
)

// Account with balance; withdrawal only allowed if balance >= amount.
type Account struct {
	ID      string
	Balance int
	Status  string
}

func (a *Account) SetState(s statemachine.State) { a.Status = string(s) }
func (a *Account) GetState() statemachine.State  { return statemachine.State(a.Status) }

type WithdrawEvent struct {
	Amount int
}

func (WithdrawEvent) GetEvent() statemachine.Event { return "withdraw" }

type DepositEvent struct{}
func (DepositEvent) GetEvent() statemachine.Event { return "deposit" }

func main() {
	sm := buildAccountMachine()
	ctx := context.Background()

	account := &Account{ID: "acc-1", Balance: 100, Status: "ACTIVE"}

	// Try to withdraw 150 (guard will fail)
	fmt.Printf("Balance: $%d, attempting withdrawal of $150\n", account.Balance)
	_, err := sm.TriggerTransition(ctx, WithdrawEvent{Amount: 150}, account)
	if err != nil {
		if errors.Is(err, statemachine.ErrGuardFailed) {
			fmt.Println("Guard failed: insufficient balance")
		} else {
			fmt.Println("error:", err)
		}
	}
	fmt.Printf("Balance: $%d, Status: %s\n\n", account.Balance, account.Status)

	// Withdraw 50 (guard passes)
	fmt.Printf("Balance: $%d, attempting withdrawal of $50\n", account.Balance)
	_, err = sm.TriggerTransition(ctx, WithdrawEvent{Amount: 50}, account)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Balance: $%d, Status: %s\n\n", account.Balance, account.Status)

	// Try to withdraw 60 (guard will fail)
	fmt.Printf("Balance: $%d, attempting withdrawal of $60\n", account.Balance)
	_, err = sm.TriggerTransition(ctx, WithdrawEvent{Amount: 60}, account)
	if err != nil {
		if errors.Is(err, statemachine.ErrGuardFailed) {
			fmt.Println("Guard failed: insufficient balance")
		}
	}
	fmt.Printf("Balance: $%d, Status: %s\n", account.Balance, account.Status)
}

func buildAccountMachine() statemachine.StateMachine {
	sm := statemachine.NewStatemachine(statemachine.EventKey{Src: "ACTIVE", Event: "withdraw"})

	// Withdrawal: only allowed if balance >= amount
	sm.AddTransition(statemachine.Transition{
		Src: []statemachine.State{"ACTIVE"}, Event: "withdraw", Dst: "ACTIVE",
		Guard: func(ctx context.Context, e statemachine.TransitionEvent, m statemachine.TransitionModel) (bool, error) {
			acc := m.(*Account)
			evt := e.(WithdrawEvent)
			// Guard: balance must be >= withdrawal amount
			allowed := acc.Balance >= evt.Amount
			if allowed {
				acc.Balance -= evt.Amount
			}
			return allowed, nil
		},
		Transition: func(ctx context.Context, e statemachine.TransitionEvent, m statemachine.TransitionModel) (statemachine.TransitionModel, error) {
			fmt.Println("Withdrawal processed")
			return nil, nil
		},
	})

	return sm
}
