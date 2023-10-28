---
title: Linux v5.19 进程调度源码解读
date: 2023-05-05 23:29:44
tags: [os, linux, cpu, thread, process, lwp]
---
``

Linux 0号进程fork 1号进程kernel_init，然后exec init process最终跳转回用户空间的全过程。

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


timer interrupt 进程切换

```c
handle_exception(arch/riscv/kernel/entry.S)
	la ra, ret_from_exception

	/* Handle interrupts */
	move a0, sp /* pt_regs */
	la a1, generic_handle_arch_irq
	jr a1
generic_handle_arch_irq()(kernel/irq/handle.c)
    riscv_intc_irq()(drivers/irqchip/irq-riscv-intc.c)
        generic_handle_domain_irq()(kernel/irq/irqdesc.c)
            handle_irq_desc()(kernel/irq/irqdesc.c)
                generic_handle_irq_desc(include/linux/irqdesc.h)
                    riscv_timer_interrupt()(drivers/clocksource/timer-riscv.c)
                    evdev->event_handler(evdev);
                        tick_handle_periodic()(kernel/time/tick-common.c)
                            tick_periodic()()(kernel/time/tick-common.c)
                                update_process_times()(kernel/time/timer.c)
                                    scheduler_tick()(kernel/sched/core.c)
                                        curr->sched_class->task_tick(rq, curr, 0);
                                        task_tick_fair()(kernel/sched/fair.c)
                                        /*
                                         * Update run-time statistics of the 'current'.
                                         */
                                        update_curr(cfs_rq);
                                        if (cfs_rq->nr_running > 1)
                                            check_preempt_tick(cfs_rq, curr);
                                            check_preempt_tick()(kernel/sched/fair.c)
                                                resched_curr()(kernel/sched/core.c)
                                                    set_tsk_need_resched()(kernel/sched/core.c)
                                                        set_tsk_thread_flag(tsk,TIF_NEED_RESCHED);
ret_from_exception(arch/riscv/kernel/entry.S)
resume_userspace(arch/riscv/kernel/entry.S)
    andi s1, s0, _TIF_WORK_MASK
    bnez s1, work_pending
work_pending(arch/riscv/kernel/entry.S)   
	/* Enter slow path for supplementary processing */
	la ra, ret_from_exception
	andi s1, s0, _TIF_NEED_RESCHED
	bnez s1, work_resched 
work_resched(arch/riscv/kernel/entry.S)   
	tail schedule
```
假设另一个进程在运行时，timer interrupt发生，进程切换回init进程，然后会切换回__switch_to，至此，进程调度变成一个死循环。