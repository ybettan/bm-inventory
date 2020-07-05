package statemachine

import (
	"testing"

	"github.com/filanov/stateswitch"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

type conditionMock struct {
	mock.Mock
}

func (c *conditionMock) eval(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (bool, error) {
	ret := c.Called(sw, args)
	return ret.Bool(0), ret.Error(1)
}

type validationFailureMock struct {
	mock.Mock
}

func (v *validationFailureMock) call(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs, failures map[string][]string) error {
	ret := v.Called(sw, args, failures)
	return ret.Error(0)
}

type stateSwitchMock struct {
	mock.Mock
}

func (s *stateSwitchMock) State() stateswitch.State {
	ret := s.Called()
	return ret.Get(0).(stateswitch.State)
}

func (s *stateSwitchMock) SetState(state stateswitch.State) error {
	ret := s.Called(state)
	return ret.Error(0)
}

func intGetter(value int) PrinterArg {
	return func(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
		return value, nil
	}
}

func stringGetter(value string) PrinterArg {
	return func(sw stateswitch.StateSwitch, args stateswitch.TransitionArgs) (interface{}, error) {
		return value, nil
	}
}

var _ = Describe("Validations test", func() {
	var sm stateswitch.StateMachine
	var fm *validationFailureMock
	var smm *stateSwitchMock
	var cm1, cm2 *conditionMock
	BeforeEach(func() {
		sm = stateswitch.NewStateMachine()
		fm = &validationFailureMock{}
		smm = &stateSwitchMock{}
		cm1 = &conditionMock{}
		cm2 = &conditionMock{}
		validations := Validations{
			Validation(cm1.eval, "hardware", Sprintf("First validation")),
			Validation(cm2.eval, "network", Sprintf("Second validation %d %s", intGetter(5), stringGetter("five"))),
		}
		sm.AddTransition(stateswitch.TransitionRule{
			TransitionType:   "test",
			SourceStates:     []stateswitch.State{"first"},
			DestinationState: "second",
			Condition:        stateswitch.Not(validations.Condition()),
			PostTransition:   MakePostValidation(validations, fm.call),
		})
		smm.On("State").Return(stateswitch.State("first"))
	})
	AfterEach(func() {
		fm.AssertExpectations(GinkgoT())
		smm.AssertExpectations(GinkgoT())
		cm1.AssertExpectations(GinkgoT())
		cm2.AssertExpectations(GinkgoT())
	})
	It("No failed validations", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(true, nil).Once()
		cm2.On("eval", mock.Anything, mock.Anything).Return(true, nil).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).To(HaveOccurred())
	})
	It("First failed", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(false, nil).Times(2)
		cm2.On("eval", mock.Anything, mock.Anything).Return(true, nil).Once()
		smm.On("SetState", stateswitch.State("second")).Return(nil).Once()
		fm.On("call", stateswitch.StateSwitch(smm), nil, map[string][]string{"hardware": {"First validation"}}).Return(nil).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).ToNot(HaveOccurred())
	})
	It("All failed", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(false, nil).Times(2)
		cm2.On("eval", mock.Anything, mock.Anything).Return(false, nil).Once()
		smm.On("SetState", stateswitch.State("second")).Return(nil).Once()
		fm.On("call", stateswitch.StateSwitch(smm), nil, map[string][]string{"hardware": {"First validation"}, "network": {"Second validation 5 five"}}).Return(nil).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).ToNot(HaveOccurred())
	})
	It("Error on first condition", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(false, errors.New("Blah")).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).To(HaveOccurred())
	})
	It("Error on second condition", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(true, nil).Once()
		cm2.On("eval", mock.Anything, mock.Anything).Return(false, errors.New("Blah")).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).To(HaveOccurred())
	})
	It("Error on validation failure", func() {
		cm1.On("eval", mock.Anything, mock.Anything).Return(true, nil).Times(2)
		cm2.On("eval", mock.Anything, mock.Anything).Return(false, nil).Times(2)
		smm.On("SetState", stateswitch.State("second")).Return(nil).Once()
		fm.On("call", stateswitch.StateSwitch(smm), nil, map[string][]string{"network": {"Second validation 5 five"}}).Return(errors.New("Blah")).Once()
		err := sm.Run("test", smm, nil)
		Expect(err).To(HaveOccurred())
	})
})

func TestSubsystem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validations tests")
}
