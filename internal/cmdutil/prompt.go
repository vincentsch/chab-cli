package cmdutil

import "github.com/vincentsch/chab-cli/internal/prompt"

// Prompt is the policy-aware prompt surface command handlers use. It keeps the
// raw prompt grammar in internal/prompt, while this type answers whether the
// active command may ask the user for interactive input.
type Prompt struct {
	prompter *prompt.Prompter
	noPrompt bool
	yes      bool
}

// Interactive reports whether command code may ask interactive questions.
func (p *Prompt) Interactive() bool {
	return p != nil && p.prompter.InteractiveIn() && !p.noPrompt
}

// InteractiveIn reports whether stdin itself is a terminal.
func (p *Prompt) InteractiveIn() bool {
	return p != nil && p.prompter.InteractiveIn()
}

// NoPrompt reports whether --no-prompt is enabled.
func (p *Prompt) NoPrompt() bool {
	return p != nil && p.noPrompt
}

// Yes reports whether --yes is enabled for supported confirmations.
func (p *Prompt) Yes() bool {
	return p != nil && p.yes
}

// Text delegates to the underlying prompt grammar.
func (p *Prompt) Text(label, prefill string) (string, error) {
	return p.prompter.Text(label, prefill)
}

// TextWithDisplay delegates to the prompt grammar while keeping the real
// blank-input default separate from its displayed form.
func (p *Prompt) TextWithDisplay(label, prefill, displayPrefill string) (string, error) {
	return p.prompter.TextWithDisplay(label, prefill, displayPrefill)
}

// Secret delegates to the underlying secret prompt.
func (p *Prompt) Secret(label string) (string, error) {
	return p.prompter.Secret(label)
}

// ReadPiped consumes non-terminal stdin.
func (p *Prompt) ReadPiped() (string, error) {
	return p.prompter.ReadPiped()
}

// Confirm prompts a yes/no question with a deny-by-default blank answer. It
// makes no policy decision; callers gate it with Interactive().
func (p *Prompt) Confirm(question string) (bool, error) {
	return p.prompter.Confirm(question, prompt.DefaultNo)
}

// Select delegates to the underlying numbered selection grammar.
func (p *Prompt) Select(label string, options []string) (int, string, error) {
	return p.prompter.Select(label, options)
}

// AbortError reports a declined or unavailable local confirmation as a local
// usage failure, before a command mutates state or calls the API.
type AbortError struct {
	Message string
}

func (e *AbortError) Error() string { return e.Message }

func (e *AbortError) ExitCode() int { return 1 }

// ConfirmDestructive applies strict destructive confirmation policy. --yes is
// accepted as explicit consent; otherwise the command must have an interactive
// prompt and a positive answer before it can continue.
func ConfirmDestructive(p *Prompt, question string) error {
	if p != nil && p.Yes() {
		return nil
	}
	if p == nil || !p.Interactive() {
		return &AbortError{Message: "this action needs confirmation; re-run with --yes to proceed without a prompt"}
	}
	ok, err := p.Confirm(question)
	if err != nil {
		return err
	}
	if !ok {
		return &AbortError{Message: "canceled; no changes were made"}
	}
	return nil
}
