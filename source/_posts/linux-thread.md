---
title: Linux v5.19 进程调度源码解读
date: 2023-05-05 23:29:44
tags: [os, linux, cpu, thread, process, lwp]
---
``

Linux 0号进程fork 1号进程kernel_init，然后exec init process最终跳转回用户空间的全过程。
timer interrupt实现切换进程待补充。

```c
_start(head.S)
_start_kernel(head.S)
start_kernel()(main.c)        
    arch_call_rest_init()(main.c)
        rest_init()(main.c)
            pid = user_mode_thread(kernel_init, NULL, CLONE_FS);(main.c)
                kernel_clone()(fork.c)
                    copy_process()(fork.c)
                        copy_thread()(process.c)
                            p->thread.ra = (unsigned long)ret_from_kernel_thread;(process.c)
                            p->thread.s[0] = (unsigned long)args->fn;
                            p->thread.s[1] = (unsigned long)args->fn_arg;
                    wake_up_new_task()(core.c)
                        activate_task()(core.c)
                            enqueue_task()(core.c)
                                p->sched_class->enqueue_task(rq, p, flags);
            schedule_preempt_disabled()(core.c)
                schedule()(core.c)
                    __schedule()(core.c)
                        next = pick_next_task(rq, prev, &rf);(core.c)
                            context_switch()(core.c)
                                switch_to()(switch_to.h)
                                    __switch_to(entry.S)
                                        li    a4,  TASK_THREAD_RA
                                        add   a4, a1, a4
                                        REG_L ra,  TASK_THREAD_RA_RA(a4)
                                        REG_L s0,  TASK_THREAD_S0_RA(a4)
                                        REG_L s1,  TASK_THREAD_S1_RA(a4)
                                        ret
ret_from_kernel_thread(entry.S)
    schedule_tail()(core.c)
    /* Call fn(arg) */
    la ra, ret_from_exception
    move a0, s1
    jr s0
kernel_init()(main.c)
    try_to_run_init_process()(main.c)
        run_init_process()(main.c)
            kernel_execve()(exec.c)
                bprm_execve()(exec.c)
                    exec_binprm()(exec.c)
                        search_binary_handler()(exec.c)
                            retval = fmt->load_binary(bprm);(exec.c)
                                load_elf_binary()(binfmt_elf).c
                                    start_thread()(process.c)
                                        regs->epc = pc;
                                        regs->sp = sp;
ret_from_exception(entry.S)    
resume_userspace
restore_all:
	REG_L  a2, PT_EPC(sp)
	REG_SC x0, a2, PT_EPC(sp)
    ...
	csrw CSR_STATUS, a0
	csrw CSR_EPC, a2
    REG_L x2,  PT_SP(sp)
    sret
```