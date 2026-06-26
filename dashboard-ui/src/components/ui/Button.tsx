import React from 'react';
import { cn } from '@/utils/cn';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';
  size?: 'sm' | 'md' | 'lg';
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', ...props }, ref) => {
    const baseStyles = 'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-gray-400 disabled:pointer-events-none disabled:opacity-50';
    
    const variants = {
      primary: 'bg-primary text-white hover:bg-blue-600 shadow-sm',
      secondary: 'bg-secondary text-white hover:bg-purple-600 shadow-sm',
      outline: 'border border-gray-700 bg-transparent hover:bg-gray-800 text-gray-200',
      ghost: 'hover:bg-gray-800 text-gray-300 hover:text-gray-100',
      danger: 'bg-danger text-white hover:bg-red-600 shadow-sm',
    };

    const sizes = {
      sm: 'h-8 px-3 text-xs',
      md: 'h-9 px-4 py-2',
      lg: 'h-10 px-8',
    };

    return (
      <button
        ref={ref}
        className={cn(baseStyles, variants[variant], sizes[size], className)}
        {...props}
      />
    );
  }
);
Button.displayName = 'Button';
