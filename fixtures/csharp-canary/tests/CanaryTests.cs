using System;
using System.IO;
using Microsoft.VisualStudio.TestTools.UnitTesting;

[TestClass]
public class CanaryTests
{
    [TestMethod]
    public void CanaryRuns()
    {
        StringWriter writer = new StringWriter();
        Console.SetOut(writer);
        Program.Main();
        Assert.AreEqual("canary", writer.ToString().Trim());
    }
}
