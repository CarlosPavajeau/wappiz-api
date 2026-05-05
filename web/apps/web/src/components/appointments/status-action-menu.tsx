"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import type {
  Appointment,
  AppointmentStatus,
  CancelledBy,
} from "@wappiz/api-client/types/appointments"
import { useState } from "react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/client-api"

import {
  getAvailableTransitions,
  getStatusConfig,
  requiresConfirmation,
  requiresReason,
} from "./appointment-utils"

const DIALOG_TITLES: Partial<Record<AppointmentStatus, string>> = {
  cancelled: "¿Cancelar esta cita?",
  completed: "¿Marcar como completada?",
  no_show: "¿Marcar como no se presentó?",
}

const DIALOG_DESCRIPTIONS: Partial<Record<AppointmentStatus, string>> = {
  cancelled:
    "Esta acción cancelará la cita de forma permanente y no se podrá deshacer.",
  completed:
    "Esta acción marcará la cita como completada y no se podrá deshacer.",
  no_show:
    "Esta acción marcará al cliente como no presentado y no se podrá deshacer.",
}

const isDestructive = (status: AppointmentStatus) => status === "cancelled"

export function StatusActionMenu({
  appointment,
}: {
  appointment: Appointment
}) {
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [pendingStatus, setPendingStatus] = useState<AppointmentStatus | null>(
    null
  )
  const [reason, setReason] = useState("")
  const [cancelledBy, setCancelledBy] = useState<CancelledBy>("customer")

  const transitions = getAvailableTransitions(appointment.status)

  const { mutate, isPending } = useMutation({
    mutationFn: (status: AppointmentStatus) =>
      api.appointments.updateStatus(appointment.id, {
        status,
        ...(requiresReason(status) && reason ? { reason } : {}),
        ...(status === "cancelled" ? { cancelled_by: cancelledBy } : {}),
      }),
    onSuccess: () => {
      setDialogOpen(false)
      setPendingStatus(null)
      setReason("")
      setCancelledBy("customer")
      queryClient.refetchQueries({ queryKey: ["appointments"] })
    },
  })

  if (transitions.length === 0) {
    return null
  }

  const [primaryStatus, ...overflowStatuses] = transitions

  const triggerAction = (status: AppointmentStatus) => {
    if (requiresConfirmation(status)) {
      setPendingStatus(status)
      setReason("")
      setCancelledBy("customer")
      setDialogOpen(true)
    } else {
      mutate(status)
    }
  }

  const handleConfirm = () => {
    if (pendingStatus) {
      mutate(pendingStatus)
    }
  }

  const handleDialogOpenChange = (open: boolean) => {
    if (!open) {
      setDialogOpen(false)
      setPendingStatus(null)
      setReason("")
      setCancelledBy("customer")
    }
  }

  return (
    <>
      {primaryStatus && (
        <Button
          key={primaryStatus}
          disabled={isPending}
          onClick={() => triggerAction(primaryStatus)}
          type="button"
        >
          {isPending && <Spinner data-icon="inline-start" />}
          Cambiar a {getStatusConfig(primaryStatus).label}
        </Button>
      )}

      {overflowStatuses.map((status) => (
        <Button
          key={status}
          disabled={isPending}
          onClick={() => triggerAction(status)}
          type="button"
          variant={isDestructive(status) ? "destructive" : "secondary"}
        >
          {isPending && <Spinner data-icon="inline-start" />}
          Cambiar a {getStatusConfig(status).label}
        </Button>
      ))}

      <AlertDialog onOpenChange={handleDialogOpenChange} open={dialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pendingStatus ? DIALOG_TITLES[pendingStatus] : ""}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {pendingStatus ? DIALOG_DESCRIPTIONS[pendingStatus] : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>

          {pendingStatus &&
            (requiresReason(pendingStatus) ||
              pendingStatus === "cancelled") && (
              <div className="flex flex-col gap-3">
                {pendingStatus === "cancelled" && (
                  <fieldset className="flex flex-col gap-1.5">
                    <legend className="text-sm font-medium">
                      Cancelado por
                    </legend>
                    <div className="flex flex-col gap-1.5">
                      <label className="flex cursor-pointer items-center gap-2 text-sm">
                        <input
                          checked={cancelledBy === "customer"}
                          className="accent-primary"
                          name="cancelled_by"
                          onChange={() => setCancelledBy("customer")}
                          type="radio"
                          value="customer"
                        />
                        Cliente
                      </label>
                      <label className="flex cursor-pointer items-center gap-2 text-sm">
                        <input
                          checked={cancelledBy === "business"}
                          className="accent-primary"
                          name="cancelled_by"
                          onChange={() => setCancelledBy("business")}
                          type="radio"
                          value="business"
                        />
                        Negocio
                      </label>
                    </div>
                  </fieldset>
                )}
                {requiresReason(pendingStatus) && (
                  <Textarea
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="Motivo (opcional)"
                    rows={3}
                    value={reason}
                  />
                )}
              </div>
            )}

          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              disabled={isPending}
              onClick={handleConfirm}
              variant={
                pendingStatus && isDestructive(pendingStatus)
                  ? "destructive"
                  : "default"
              }
            >
              {isPending ? <Spinner /> : "Confirmar"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
