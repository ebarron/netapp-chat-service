import { SimpleGrid, Text, Anchor } from '@mantine/core';
import type { PropertiesData } from './chartTypes';
import { applyQualifier } from './ObjectDetailBlock';

interface PropertiesSectionProps {
  data: PropertiesData;
  onAction?: (message: string) => void;
  /** Card-level qualifier from the parent object-detail block. */
  cardQualifier?: string;
}

export function PropertiesSection({ data, onAction, cardQualifier }: PropertiesSectionProps) {
  const cols = data.columns ?? 2;

  return (
    <SimpleGrid cols={cols} spacing="xs" verticalSpacing={4}>
      {data.items.map((item, i) => (
        <div key={i}>
          <Text fz="xs" c="dimmed" lh={1.2}>
            {item.label}
          </Text>
          {item.link ? (
            <Anchor
              fz="sm"
              fw={500}
              underline="hover"
              onClick={() => onAction?.(applyQualifier(item.link!, item.qualifier, cardQualifier))}
              style={{ cursor: 'pointer' }}
              c={item.color}
            >
              {item.value}
            </Anchor>
          ) : (
            <Text fz="sm" fw={500} c={item.color}>
              {item.value}
            </Text>
          )}
        </div>
      ))}
    </SimpleGrid>
  );
}
