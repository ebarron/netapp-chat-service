import { useState } from 'react';
import { Stack, Group, Button, TextInput, Select, Switch, Divider, SimpleGrid } from '@mantine/core';
import type { ActionFormData } from './chartTypes';

interface ActionFormBlockProps {
  data: ActionFormData;
  onAction?: (message: string) => void;
  readOnly?: boolean;
}

export function ActionFormBlock({ data, onAction, readOnly }: ActionFormBlockProps) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const field of data.fields) {
      init[field.key] = field.defaultValue ?? '';
    }
    return init;
  });

  const requiredMissing = data.fields.some(
    (f) => f.required && !values[f.key]?.trim()
  );

  // Form submits run a tool, so they're treated as read-write by default and
  // disabled in read-only mode. A submit can opt out (requiresReadWrite:false)
  // when it's read-only-safe — e.g. a picker that only re-renders a dashboard.
  const writeGated = data.submit.requiresReadWrite ?? true;
  const lockedReadOnly = !!readOnly && writeGated;

  const handleSubmit = () => {
    if (requiredMissing) return;
    const merged: Record<string, unknown> = { ...data.submit.params };
    const checkboxKeys = new Set(data.fields.filter((f) => f.type === 'checkbox').map((f) => f.key));
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
    onAction?.(`Run ${data.submit.tool} with ${paramStr}`);
  };

  const setField = (key: string, val: string) =>
    setValues((prev) => ({ ...prev, [key]: val }));

  // Render every field in the same responsive grid; checkboxes flow inline
  // with selects/text inputs so the form stays compact.
  const allFields = data.fields;

  const renderField = (field: typeof data.fields[number]) =>
    field.type === 'select' ? (
      <Select
        key={field.key}
        label={field.label}
        placeholder={field.placeholder}
        data={[...new Set(field.options ?? [])]}
        value={values[field.key] || null}
        onChange={(v) => setField(field.key, v ?? '')}
        clearable
        size="sm"
        comboboxProps={{ transitionProps: { duration: 0 } }}
      />
    ) : field.type === 'checkbox' ? (
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
      <Divider />
      <Group justify="space-between" align="center" wrap="nowrap">
        <Button
          size="sm"
          disabled={requiredMissing || lockedReadOnly}
          onClick={handleSubmit}
        >
          {data.submit.label}
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
    </Stack>
  );
}
