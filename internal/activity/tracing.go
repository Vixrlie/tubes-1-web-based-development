package activity

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func StartTracing(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "pathgres"))),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer("pathgres").Start(ctx, name)
}

func EndSpan(span trace.Span, failed bool) {
	if failed {
		span.SetStatus(codes.Error, "operation failed")
	}
	span.End()
}

// TraceHTTP is installed outside recovery so recovered panics report HTTP 500.
func TraceHTTP(next http.Handler) http.Handler {
	logger := New()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("pathgres").Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer))
		response := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			status := response.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(ctx).RoutePattern()
			if route != "" {
				span.SetName(r.Method + " " + route)
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			span.SetAttributes(attribute.String("http.route", route))
			requestStatus := StatusSuccess
			if status >= 400 {
				requestStatus = StatusFailed
			}
			logger.Log(ctx, Event{Action: "http.request", Status: requestStatus, Detail: map[string]string{
				"method": r.Method, "route": route, "duration": time.Since(start).String(),
			}})
			EndSpan(span, status >= 500)
		}()
		next.ServeHTTP(response, r.WithContext(ctx))
	})
}
