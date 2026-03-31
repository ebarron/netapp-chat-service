import { useState, useMemo } from 'react';
import { Group, Button, TextInput, Select, Switch } from '@mantine/core';
import type { ActionFormData, ActionFormField } from './chartTypes';

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

  const initialValues = useMemo(() => {
    const init: Record<string, string> = {};
    for (const field of data.fields) {
      init[field.key] = field.defaultValue ?? '';
    }
    return init;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const requiredMissing = data.fields.some(
    (f) => f.required && !values[f.key]?.trim()
  );

  const recheckDirty = data.recheck?.fields.some(
    (key) => values[key] !== initialValues[key]
  ) ?? false;

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

  const handleRecheck = () => {
    if (!data.recheck) return;
    const all: Record<string, string> = { ...data.submit.params as Record<string, string>, ...values };
    const msg = data.recheck.message.replace(/\{(\w+)\}/g, (_, key) => all[key] ?? '');
    onAction?.(msg);
  };

  const setField = (key: string, val: string) =>
    setValues((prev) => ({ ...prev, [key]: val }));

  return (
    <Group gap="xs" align="flex-end" role="group" aria-label="Provisioning form">
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
            style={{ minWidth: 180 }}
            comboboxProps={{ transitionProps: { duration: 0 } }}
          />
        ) : field.type === 'checkbox' ? (
          <Switch
            key={field.key}
            label={field.label}
            checked={values[field.key] === 'true'}
            onChange={(e) => setField(field.key, e.currentTarget.checked ? 'true' : 'false')}
            size="sm"
          />
        ) : (
          <TextInput
            key={field.key}
            label={field.label}
            placeholder={field.placeholder}
            value={values[field.key]}
            onChange={(e) => {
              const val = e.currentTarget.value;
              setField(field.key, val);
            }}
            size="sm"
            style={{ minWidth: 180 }}
          />
        )
      )}
      <Button
        size="compact-sm"
        disabled={requiredMissing || readOnly}
        onClick={handleSubmit}
      >
        {data.submit.label}
      </Button>
      {data.recheck && recheckDirty && (
        <Button
          size="compact-sm"
          variant="light"
          onClick={handleRecheck}
        >
          {data.recheck.label}
        </Button>
      )}
      {data.secondary && (
        <Button
          size="compact-sm"
          variant="outline"
          onClick={() => onAction?.(data.secondary!.message)}
        >
          {data.secondary.label}
        </Button>
      )}
    </Group>
  );
}
