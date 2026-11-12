//go:build windows

package main

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tarefaDoOuvido segura os navegadores escondidos: quando este programa sai, de qualquer
// jeito, o Windows fecha a tarefa e encerra o que estiver dentro dela.
var tarefaDoOuvido windows.Handle

// amarrarNavegador liga a vida do navegador escondido a deste programa (Windows), com
// um objeto de tarefa marcado para encerrar os processos quando for fechado.
func amarrarNavegador(cmd *exec.Cmd) (depoisDeIniciar func(), err error) {
	return func() {
		if cmd.Process == nil {
			return
		}
		if tarefaDoOuvido == 0 {
			tarefa, err := windows.CreateJobObject(nil, nil)
			if err != nil {
				return
			}
			info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
			info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			if _, err := windows.SetInformationJobObject(tarefa, windows.JobObjectExtendedLimitInformation,
				uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
				_ = windows.CloseHandle(tarefa)
				return
			}
			tarefaDoOuvido = tarefa
		}
		processo, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			return
		}
		defer windows.CloseHandle(processo)
		_ = windows.AssignProcessToJobObject(tarefaDoOuvido, processo)
	}, nil
}
