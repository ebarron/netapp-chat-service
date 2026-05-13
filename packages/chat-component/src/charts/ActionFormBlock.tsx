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

  // Split inputs from the monitoring switch so the switch can sit alone on the
  // last row and never get visually paired with an unrelated select.
  const inputFields = data.fields.filter((f) => f.type !== 'checkbox');
  const switchFields = data.fields.filter((f) => f.type === 'checkbox');

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
      {inputFields.length > 0 && (
        <SimpleGrid cols={{ base: 1, xs: inputFields.length > 1 ? 2 : 1 }} spacing="xs" verticalSpacing="xs">
          {inputFields.map(renderField)}
        </SimpleGrid>
      )}
      {switchFields.length > 0 && (
        <Stack gap="xs">{switchFields.map(renderField)}</Stack>
      )}
      <Divider />
      <Group justify="space-between" align="center" wrap="nowrap">
        <Button
          size="sm"
          disabled={requiredMissing || readOnly}
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
