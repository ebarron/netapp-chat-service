import { useState } from 'react';
import { Stack, Group, Button, TextInput, Select, Switch, Divider } from '@mantine/core';
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

  return (
    <Stack gap="sm" role="group" aria-label="Provisioning form">
      <Stack gap="xs">
        {data.fields.map((field) =>
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
          )
        )}
      </Stack>
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
