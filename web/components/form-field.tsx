"use client";

import { useId } from "react";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

type FormFieldProps = {
  label: string;
  required?: boolean;
  error?: string;
  className?: string;
  children?: React.ReactNode;
} & (
  | {
      type?: "text" | "email" | "password" | "number" | "tel" | "url";
      placeholder?: string;
      value?: string;
      onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
      disabled?: boolean;
      inputProps?: React.ComponentPropsWithoutRef<"input">;
    }
  | {
      type: "textarea";
      placeholder?: string;
      value?: string;
      onChange?: (e: React.ChangeEvent<HTMLTextAreaElement>) => void;
      disabled?: boolean;
      inputProps?: React.ComponentPropsWithoutRef<"textarea">;
    }
  | {
      type: "select";
      placeholder?: string;
      value?: string;
      onChange?: (e: React.ChangeEvent<HTMLSelectElement>) => void;
      disabled?: boolean;
      children: React.ReactNode;
      inputProps?: React.ComponentPropsWithoutRef<"select">;
    }
);

function FormField(props: FormFieldProps) {
  const generatedId = useId();
  const { label, required, error, className } = props;

  const inputId = generatedId;
  const errorId = error ? `${inputId}-error` : undefined;

  return (
    <div className={cn("space-y-1.5", className)}>
      <Label htmlFor={inputId} required={required}>
        {label}
      </Label>

      {props.type === "textarea" ? (
        <Textarea
          id={inputId}
          placeholder={props.placeholder}
          value={props.value}
          onChange={props.onChange}
          disabled={props.disabled}
          error={!!error}
          aria-describedby={errorId}
          aria-invalid={!!error}
          {...(props.inputProps as React.ComponentPropsWithoutRef<"textarea">)}
        />
      ) : props.type === "select" ? (
        <Select
          id={inputId}
          value={props.value}
          onChange={props.onChange}
          disabled={props.disabled}
          error={!!error}
          aria-describedby={errorId}
          aria-invalid={!!error}
          {...(props.inputProps as React.ComponentPropsWithoutRef<"select">)}
        >
          {props.children}
        </Select>
      ) : (
        <Input
          id={inputId}
          type={props.type ?? "text"}
          placeholder={props.placeholder}
          value={props.value}
          onChange={
            props.onChange as (e: React.ChangeEvent<HTMLInputElement>) => void
          }
          disabled={props.disabled}
          error={!!error}
          aria-describedby={errorId}
          aria-invalid={!!error}
          {...(props.inputProps as React.ComponentPropsWithoutRef<"input">)}
        />
      )}

      {error && (
        <p id={errorId} className="text-sm text-red-600" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

export { FormField };
export type { FormFieldProps };
