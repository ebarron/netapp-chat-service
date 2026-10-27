import { useState } from 'react';
import { Stack, Group, Button, TextInput, Select, Switch, Divider, SimpleGrid } from '@mantine/core';
import type { ActionFormData, ActionFormField, ActionFormSelectOption } from './chartTypes';

interface ActionFormBlockProps {
  data: ActionFormData;
  onAction?: (message: string) => void;
  readOnly?: boolean;
}

interface SelectOption {
  value: string;
  label: string;
  dependsOn?: ActionFormSelectOption['dependsOn'];
}

function normalizedOptions(field: ActionFormField): SelectOption[] {
  return (field.options ?? []).map((option) => (
    typeof option === 'string'
      ? { value: option, label: option }
      : { value: option.value, label: option.label ?? option.value, dependsOn: option.dependsOn }
  ));
}

function availableOptions(field: ActionFormField, values: Record<string, string>): SelectOption[] {
  const seen = new Set<string>();
  return normalizedOptions(field).filter((option) => {
    const matches = Object.entries(option.dependsOn ?? {}).every(([key, accepted]) => (
      Array.isArray(accepted)
        ? accepted.includes(values[key] ?? '')
        : accepted === (values[key] ?? '')
    ));
    if (!matches || seen.has(option.value)) return false;
    seen.add(option.value);
    return true;
  });
}

function reconcileSelectValues(
  fields: ActionFormField[],
  current: Record<string, string>,
): Record<string, string> {
  const next = { ...current };
  for (const field of fields) {
    if (field.type !== 'select') continue;
    const options = availableOptions(field, next);
    if (options.some((option) => option.value === next[field.key])) continue;
    next[field.key] = field.autoSelect && options.length > 0 ? options[0].value : '';
  }
  return next;
}

export function ActionFormBlock({ data, onAction, readOnly }: ActionFormBlockProps) {
  const fields = Array.isArray(data.fields)
    ? data.fields.filter((field) => (
        typeof field === 'object'
        && field !== null
        && typeof field.key === 'string'
        && typeof field.label === 'string'
      ))
    : [];
  const submit = data.submit && typeof data.submit === 'object' ? data.submit : null;
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const field of fields) {
      init[field.key] = field.defaultValue ?? '';
    }
    return reconcileSelectValues(fields, init);
  });

  const requiredMissing = fields.some(
    (f) => f.required && !values[f.key]?.trim()
  );

  // Form submits run a tool, so they're treated as read-write by default and
  // disabled in read-only mode. A submit can opt out (requiresReadWrite:false)
  // when it's read-only-safe — e.g. a picker that only re-renders a dashboard.
  const writeGated = submit?.requiresReadWrite ?? true;
  const lockedReadOnly = !!readOnly && writeGated;

  const handleSubmit = () => {
    if (requiredMissing || !submit) return;
    const merged: Record<string, unknown> = { ...submit.params };
    const checkboxKeys = new Set(fields.filter((f) => f.type === 'checkbox').map((f) => f.key));
    for (const [k, v] of Object.entries(values)) {
      if (checkboxKeys.has(k)) {
        if (v === 'true') merged[k] = v;
      } else if (v.trim()) {
        merged[k] = v.trim();
      }
    }
    const paramStr = Object.entries(merged)
      .map(([k, v]) => `${k}=${v}`)
      .join(', ');
    onAction?.(`Run ${submit.tool} with ${paramStr}`);
  };

  const setField = (key: string, val: string) =>
    setValues((prev) => reconcileSelectValues(fields, { ...prev, [key]: val }));

  // Render every field in the same responsive grid; checkboxes flow inline
  // with selects/text inputs so the form stays compact.
  const allFields = fields;

  const renderField = (field: typeof fields[number]) => {
    if (field.type === 'select') {
      const available = availableOptions(field, values);
      const options = available.map(({ value, label }) => ({ value, label }));
      // Mantine caches Combobox option labels. Remount this one Select when its
      // dependency branch changes so an identical submitted value on two
      // branches (for example the "default" policy on two clusters) keeps the
      // label and remains selectable.
      const optionSetKey = available
        .map(({ value, label, dependsOn }) => `${value}\u0000${label}\u0000${JSON.stringify(dependsOn ?? {})}`)
        .join('\u0001');
      return (
        <Select
          key={`${field.key}:${optionSetKey}`}
          label={field.label}
          placeholder={field.placeholder}
          data={options}
          value={values[field.key] || null}
          onChange={(v) => setField(field.key, v ?? '')}
          clearable={!field.autoSelect}
          size="sm"
          comboboxProps={{ transitionProps: { duration: 0 } }}
        />
      );
    }
    return field.type === 'checkbox' ? (
      <Switch
        key={field.key}
        label={field.label}
        checked={values[field.key] === 'true'}
        onChange={(e) => setField(field.key, e.currentTarget.checked ? 'true' : 'false')}
        size="sm"
        mt={4}
      />
    ) : (
      <TextInput
        key={field.key}
        label={field.label}
        placeholder={field.placeholder}
        value={values[field.key]}
        onChange={(e) => setField(field.key, e.currentTarget.value)}
        size="sm"
      />
    );
  };

  return (
    <Stack gap="sm" role="group" aria-label="Provisioning form" maw={560}>
      {allFields.length > 0 && (
        <SimpleGrid
          cols={{
            base: 1,
            xs: Math.min(allFields.length, 2),
            sm: Math.min(allFields.length, 3),
          }}
          spacing="xs"
          verticalSpacing="xs"
        >
          {allFields.map(renderField)}
        </SimpleGrid>
      )}
      {submit && (
        <>
          <Divider />
          <Group justify="space-between" align="center" wrap="nowrap">
            <Button
              size="sm"
              disabled={requiredMissing || lockedReadOnly}
              onClick={handleSubmit}
            >
              {submit.label}
            </Button>
            {data.secondary && (
              <Button
                size="compact-sm"
                variant="subtle"
                color="gray"
                onClick={() => onAction?.(data.secondary!.message)}
              >
                {data.secondary.label}
              </Button>
            )}
          </Group>
        </>
      )}
    </Stack>
  );
}
