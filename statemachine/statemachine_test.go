package statemachine

import (
	"context"
	"fmt"
	"testing"
)

type TestStruct struct {
	Id     string
	Status string
}

func (p *TestStruct) SetState(s State) {
	p.Status = string(s)
}

func (p *TestStruct) GetState() State {
	return State(p.Status)
}

type onMelt struct {
}

func (receiver *onMelt) GetEvent() Event {
	return "onMelt"
}

type onVapourise struct {
}

func (receiver *onVapourise) GetEvent() Event {
	return "onVapourise"
}

type onCondensation struct {
}

func (receiver *onCondensation) GetEvent() Event {
	return "onCondensation"
}

type onFreeze struct {
}

func (receiver *onFreeze) GetEvent() Event {
	return "onFreeze"
}

func Test_stateMachine_TriggerTransition(t *testing.T) {
	type fields struct {
		startEvent  EventKey
		transitions map[EventKey]Transition
	}
	type args struct {
		ctx context.Context
		e   TransitionEvent
		t   TransitionModel
	}

	ek, trs := getTestData()
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "testing SOLID -> onMelt",
			fields: fields{
				startEvent:  ek,
				transitions: trs,
			},
			args: args{
				ctx: context.Background(),
				e:   &onMelt{},
				t: &TestStruct{
					Id:     "t_123",
					Status: "SOLID",
				},
			},
			wantErr: false,
		},
		{
			name: "testing LIQUID -> onVapourise",
			fields: fields{
				startEvent:  ek,
				transitions: trs,
			},
			args: args{
				ctx: context.Background(),
				e:   &onVapourise{},
				t: &TestStruct{
					Id:     "t_123",
					Status: "LIQUID",
				},
			},
			wantErr: false,
		},
		{
			name: "testing GAS -> onCondensation",
			fields: fields{
				startEvent:  ek,
				transitions: trs,
			},
			args: args{
				ctx: context.Background(),
				e:   &onCondensation{},
				t: &TestStruct{
					Id:     "t_123",
					Status: "GAS",
				},
			},
			wantErr: false,
		},
		{
			name: "testing LIQUID -> onFreeze",
			fields: fields{
				startEvent:  ek,
				transitions: trs,
			},
			args: args{
				ctx: context.Background(),
				e:   &onFreeze{},
				t: &TestStruct{
					Id:     "t_123",
					Status: "LIQUID",
				},
			},
			wantErr: false,
		},
		{
			name: "testing LIQUID -> onMelt",
			fields: fields{
				startEvent:  ek,
				transitions: trs,
			},
			args: args{
				ctx: context.Background(),
				e:   &onMelt{},
				t: &TestStruct{
					Id:     "t_123",
					Status: "LIQUID",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &stateMachine{
				startEvent:  tt.fields.startEvent,
				transitions: tt.fields.transitions,
			}
			if _, err := s.TriggerTransition(tt.args.ctx, tt.args.e, tt.args.t); (err != nil) != tt.wantErr {
				t.Errorf("TriggerTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func getTestData() (EventKey, map[EventKey]Transition) {
	sm := NewStatemachine(EventKey{
		Src:   "SOLID",
		Event: "onMelt",
	})

	// initialize statemachine
	sm.AddTransition(Transition{
		Src:        []State{"SOLID"},
		Event:      "onMelt",
		Dst:        "LIQUID",
		Transition: onEvent(),
	})

	sm.AddTransition(Transition{
		Src:              []State{"LIQUID"},
		Event:            "onVapourise",
		Dst:              "GAS",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})

	sm.AddTransition(Transition{
		Src:              []State{"GAS"},
		Event:            "onCondensation",
		Dst:              "LIQUID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})

	sm.AddTransition(Transition{
		Src:              []State{"LIQUID"},
		Event:            "onFreeze",
		Dst:              "SOLID",
		BeforeTransition: onBeforeEvent(),
		Transition:       onEvent(),
		AfterTransition:  onAfterEvent(),
		OnSuccess:        onSuccess(),
		OnFailure:        onFailure(),
	})

	return sm.GetTransitions()
}

func onBeforeEvent() BeforeTransitionHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("before")
		return nil, nil
	}
}

func onEvent() TransitionHandler {
	return func(context.Context, TransitionEvent, TransitionModel) (TransitionModel, error) {
		fmt.Println("during")
		return nil, nil
	}
}

func onAfterEvent() AfterTransitionHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("after")
		return nil, nil
	}
}

func onSuccess() OnSuccessHandler {
	return func(context.Context, TransitionModel) (TransitionModel, error) {
		fmt.Println("success")
		return nil, nil
	}
}

func onFailure() OnFailureHandler {
	return func(ctx context.Context, t TransitionModel, s Error, err error) (TransitionModel, error) {
		fmt.Println("failure : ", err)
		return nil, err
	}
}
