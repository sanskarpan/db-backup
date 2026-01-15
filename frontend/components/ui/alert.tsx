import * as React from 'react'
import { clsx } from 'clsx'

export interface AlertProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'destructive' | 'success' | 'warning'
}

export function Alert({ className, variant = 'default', ...props }: AlertProps) {
  const variantClasses = {
    default: 'bg-blue-50 text-blue-900 border-blue-200',
    destructive: 'bg-red-50 text-red-900 border-red-200',
    success: 'bg-green-50 text-green-900 border-green-200',
    warning: 'bg-yellow-50 text-yellow-900 border-yellow-200',
  }

  return (
    <div
      role="alert"
      className={clsx(
        'relative w-full rounded-lg border p-4',
        variantClasses[variant],
        className
      )}
      {...props}
    />
  )
}

export interface AlertTitleProps extends React.HTMLAttributes<HTMLHeadingElement> {}

export function AlertTitle({ className, ...props }: AlertTitleProps) {
  return (
    <h5
      className={clsx('mb-1 font-medium leading-none tracking-tight', className)}
      {...props}
    />
  )
}

export interface AlertDescriptionProps extends React.HTMLAttributes<HTMLParagraphElement> {}

export function AlertDescription({ className, ...props }: AlertDescriptionProps) {
  return (
    <div
      className={clsx('text-sm [&_p]:leading-relaxed', className)}
      {...props}
    />
  )
}
