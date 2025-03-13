/*
 *  © Copyright Kaleido, Inc. 2024. The materials in this file constitute the "Pre-Existing IP,"
 *  "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 *  under a limited, perpetual license only, subject to the terms of the applicable license
 *  agreement between you and Kaleido, Inc.  All other rights reserved.
 */

package org.lfdt.labs.paladin.diagnostics;

import static java.lang.System.out;

import javax.management.InstanceNotFoundException;
import javax.management.MBeanException;
import javax.management.MBeanServer;
import javax.management.MalformedObjectNameException;
import javax.management.ObjectName;
import javax.management.ReflectionException;
import java.lang.management.ManagementFactory;

/**
 * Provide virtual machine diagnostics using DiagnosticCommandMBean
 * that comes with the Oracle HotSpot JVM.
 *
 * Credit (Apache 2.0): <a href="https://github.com/dustinmarx/javautilities/blob/master/dustin/utilities/diagnostics/VirtualMachineDiagnostics.java">...</a>
 */
public class VirtualMachineDiagnostics
{
    /** Object Name of DiagnosticCommandMBean. */
    public final static String DIAGNOSTIC_COMMAND_MBEAN_OBJECT_NAME =
            "com.sun.management:type=DiagnosticCommand";

    /** My MBean Server. */
    private final MBeanServer server = ManagementFactory.getPlatformMBeanServer();

    /** Platform MBean Server. */
    private final ObjectName objectName;

    /**
     * Create an instance of me with the provided object name.
     *
     * @param newObjectName ObjectName associated with
     *    DiagnosticCommand MBean.
     */
    private VirtualMachineDiagnostics(final ObjectName newObjectName)
    {
        this.objectName = newObjectName;
    }

    /**
     * Only publicly available method for instantiating an instance
     * of me.
     *
     * @return An instance of me.
     */
    public static VirtualMachineDiagnostics newInstance()
    {
        try
        {
            final ObjectName objectName = new ObjectName(DIAGNOSTIC_COMMAND_MBEAN_OBJECT_NAME);
            return new VirtualMachineDiagnostics(objectName);
        }
        catch (MalformedObjectNameException badObjectNameEx)
        {
            throw new RuntimeException(
                    "Unable to create an ObjectName and so unable to create instance of VirtualMachineDiagnostics");
        }
    }

    /**
     * Provide class statistics as single String.
     *
     * This is only supported when {@code -XX:+UnlockDiagnosticVMOptions} is enabled.
     *
     * @return Single string containing formatted class statistics.
     */
    public String getClassStatistics()
    {
        return invokeNoStringArgumentsCommand("gcClassStats", "GC Class Statistics");
    }

    /**
     * Provide class histogram as single String.
     *
     * @return Single string containing formatted class histogram.
     */
    public String getHistogram()
    {
        return invokeNoStringArgumentsCommand("gcClassHistogram", "GC Class Histogram");
    }

    /**
     * Provide thread dump as single String.
     *
     * @return Single string containing formatted thread dump.
     */
    public String getThreadDump()
    {
        return invokeNoStringArgumentsCommand("threadPrint", "Thread Dump");
    }

    /**
     * Provide virtual machine uptime as single String.
     *
     * @return Single string containing virtual machine uptime.
     */
    public String getVirtualMachineUptime()
    {
        return invokeNoStringArgumentsCommand("vmUptime", "Virtual Machine Uptime");
    }

    /**
     * Provide list of supported operations (help).
     *
     * @return Single string containing names of supported operations.
     */
    public String getAvailableOperations()
    {
        return invokeNoStringArgumentsCommand("help", "Help (List Commands)");
    }

    /**
     * Provide a String representing the Virtual Machine flags.
     *
     * @return String containing the virtual machine flags.
     */
    public String getVirtualMachineFlags()
    {
        return invokeNoStringArgumentsCommand("vmFlags", "Determine VM flags");
    }

    /**
     * Provide String representing active/current garbage collector.
     *
     * @return String representation of current garbage collector
     *    ("Parallel/Throughput", "Concurrent Mark Sweep (CMS)",
     *    "Garbage First", or "UNDETERMINED").
     */
    public String determineGarbageCollector()
    {
        String garbageCollector;
        final String vmFlags = getVirtualMachineFlags();
        if (vmFlags.contains("+UseParallelGC") || vmFlags.contains("+UseParallelOldGC"))
        {
            garbageCollector = "Parallel/Throughput";
        }
        else if (vmFlags.contains("+UseConcMarkSweepGC"))
        {
            garbageCollector = "Concurrent Mark Sweep (CMS)";
        }
        else if (vmFlags.contains("+UseG1GC"))
        {
            garbageCollector = "Garbage First";
        }
        else
        {
            garbageCollector = "UNDETERMINED";
        }
        return garbageCollector;
    }

    /**
     * Invoke operation on the DiagnosticCommandMBean that accepts
     *    String array argument but does not require any String
     *    argument and returns a String.
     *
     * @param operationName Name of operation on DiagnosticCommandMBean.
     * @param operationDescription Description of operation being invoked
     *    on the DiagnosticCommandMBean.
     * @return String returned by DiagnosticCommandMBean operation.
     */
    private String invokeNoStringArgumentsCommand(
            final String operationName, final String operationDescription)
    {
        String result;
        try
        {
            result = (String) server.invoke(objectName, operationName, new Object[] {null}, new String[]{String[].class.getName()});
        }
        catch (InstanceNotFoundException | ReflectionException | MBeanException exception)
        {
            result = "ERROR: Unable to access '" + operationDescription + "' - " + exception;
        }
        return result;
    }

    public static void main(final String[] arguments)
    {
        final VirtualMachineDiagnostics instance = VirtualMachineDiagnostics.newInstance();
        out.println("VM Flags:\n\t" + instance.getVirtualMachineFlags());
        out.println("Garbage Collector: " + instance.determineGarbageCollector());
        out.println("Histogram:\n" + instance.getHistogram());
        out.println("Thread Stack:\n" + instance.getThreadDump());
        out.println("VM Uptime: " + instance.getVirtualMachineUptime());
        out.println("Class Statistics: " + instance.getClassStatistics());
        out.println("Supported Operations:\n" + instance.getAvailableOperations());
    }
}
