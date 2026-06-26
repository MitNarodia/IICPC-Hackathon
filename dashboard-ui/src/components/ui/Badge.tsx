import React from 'react';
import { cn } from '@/utils/cn';

interface BadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'outline';
}

export function Badge({ className, variant = 'default', ...props }: BadgeProps) {
  const variants = {
    default: 'border-transparent bg-gray-800 text-gray-100',
    success: 'border-transparent bg-green-500/10 text-green-400',
    warning: 'border-transparent bg-yellow-500/10 text-yellow-400',
    danger: 'border-transparent bg-red-500/10 text-red-400',
    outline: 'text-gray-100',
  };

  return (
    <div
      className={cn(
        'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-2',
        variants[variant],
        className
      )}
      {...props}
    />
  );
}
