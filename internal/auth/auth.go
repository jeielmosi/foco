package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"foco/internal/config"
	"foco/internal/enums"
	"foco/internal/prompt"
)

type Options struct {
	Schema   enums.Schema
	Endpoint string
	Model    string
	Stdin    bool
}

func Authenticate(ctx context.Context, options Options, stdin io.Reader, stdout io.Writer, ui prompt.Prompter) error {
	if err := ctx.Err(); err != nil {
		return prompt.ErrCancelled
	}
	var credentials config.Credentials
	if options.Schema != enums.UnknownSchema {
		if !options.Schema.IsValid() {
			return config.ErrInvalidSchema
		}
		endpoint, err := config.ValidateEndpoint(options.Endpoint)
		if err != nil {
			return err
		}
		model, err := config.ValidateModel(options.Model)
		if err != nil {
			return err
		}
		if !options.Stdin {
			return errors.New(pipedKeyMessage())
		}
		key, err := readKey(ctx, stdin)
		if err != nil {
			return err
		}
		credentials.Schema = options.Schema
		credentials.Endpoint = endpoint
		credentials.Model = model
		credentials.APIKey = key
	} else {
		if ui == nil {
			return errors.New(pipedKeyMessage())
		}
		selected, err := ui.Select("Choose an API schema", schemaOptions(), 0)
		if err != nil {
			return err
		}
		schema, validSchema := enums.NewSchema(selected)
		if !validSchema {
			return config.ErrInvalidSchema
		}
		endpoint, err := ui.Input("Exact API endpoint URL", "https://api.example.com/path", validateEndpoint)
		if err != nil {
			return err
		}
		normalizedEndpoint, err := config.ValidateEndpoint(endpoint)
		if err != nil {
			return err
		}
		model, err := ui.Input("Model ID", "model-id", validateModel)
		if err != nil {
			return err
		}
		normalizedModel, err := config.ValidateModel(model)
		if err != nil {
			return err
		}
		keyPrompt := fmt.Sprintf("Paste the API key for %s", config.SchemaLabel(schema))
		key, err := ui.Password(keyPrompt, validateKey)
		if err != nil {
			return err
		}
		normalizedKey, err := config.ValidateKey(key)
		if err != nil {
			return err
		}
		credentials.Schema = schema
		credentials.Endpoint = normalizedEndpoint
		credentials.Model = normalizedModel
		credentials.APIKey = normalizedKey
	}
	profile, err := config.Resolve(credentials)
	if err != nil {
		return err
	}
	path, err := config.CredentialsPath()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return prompt.ErrCancelled
	}
	if err := config.Save(path, credentials); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s key saved. Run foco doctor to verify access.\n", profile.Label)
	return nil
}

func schemaOptions() []prompt.Option {
	ids := config.SchemaIDs()
	options := make([]prompt.Option, 0, len(ids))
	for _, id := range ids {
		options = append(options, prompt.Option{Value: id.String(), Label: config.SchemaLabel(id)})
	}
	return options
}

func pipedKeyMessage() string {
	ids := config.SchemaIDs()
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = id.String()
	}
	return "Use auth --schema " + strings.Join(values, "|") + " --endpoint URL --model ID --stdin to read a piped key."
}

func validateEndpoint(value string) error {
	_, err := config.ValidateEndpoint(value)
	return err
}

func validateModel(value string) error {
	_, err := config.ValidateModel(value)
	return err
}

func validateKey(value string) error {
	_, err := config.ValidateKey(value)
	return err
}

func readKey(ctx context.Context, stdin io.Reader) (string, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(stdin, 8193))
		done <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		if closer, ok := stdin.(io.Closer); ok {
			_ = closer.Close()
		}
		return "", prompt.ErrCancelled
	case outcome := <-done:
		if ctx.Err() != nil {
			if closer, ok := stdin.(io.Closer); ok {
				_ = closer.Close()
			}
			return "", prompt.ErrCancelled
		}
		if outcome.err != nil {
			return "", outcome.err
		}
		if len(outcome.data) > 8192 {
			return "", errors.New("Auth input exceeds 8 KiB.")
		}
		return config.ValidateKey(string(outcome.data))
	}
}
